package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	stripego "github.com/stripe/stripe-go/v85"
	"golang.org/x/crypto/bcrypt"

	"github.com/spencercornish/booklab/internal/db"
	emailsvc "github.com/spencercornish/booklab/internal/email"
	"github.com/spencercornish/booklab/internal/stripe/stripetest"
)

// A burst of concurrent wrong-password logins must not get more than loginRateLimitMax
// password checks: each attempt is recorded before the limit is checked.
func TestAdminLogin_concurrentBurstIsRateLimited(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)

	// A real cost keeps each password check slow, as in production.
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateAdminUser(ctx, "admin", string(hash)); err != nil {
		t.Fatal(err)
	}

	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger())
	srv := New(testConfig(), q, stripetest.New(), emailSvc, nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	const burst = 40
	var (
		mu       sync.Mutex
		statuses = map[int]int{}
	)
	// Two bursts: guesses from both together must stay within the limit.
	for round := 0; round < 2; round++ {
		var wg sync.WaitGroup
		for i := 0; i < burst; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				body := fmt.Sprintf(`{"username":"admin","password":"guess-%d"}`, i)
				req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/admin/login", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				// Spoofed client IPs must not help: the test server's peer is not a trusted proxy.
				req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i))
				req.Header.Set("True-Client-IP", fmt.Sprintf("203.0.113.%d", i))
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Error(err)
					return
				}
				resp.Body.Close()
				mu.Lock()
				statuses[resp.StatusCode]++
				mu.Unlock()
			}(i)
		}
		wg.Wait()
	}

	// Concurrent attempts count against each other before any is removed, so a burst may
	// be refused entirely; what matters is that it never gets more than the limit.
	if got := statuses[http.StatusUnauthorized]; got > loginRateLimitMax {
		t.Fatalf("%d password checks in two bursts of %d, want at most %d (statuses %v)", got, burst, loginRateLimitMax, statuses)
	}
	if statuses[http.StatusUnauthorized]+statuses[http.StatusTooManyRequests] != 2*burst {
		t.Fatalf("unexpected statuses %v", statuses)
	}
}

func clearNewCustomerRestriction(t *testing.T, ctx context.Context, q *db.Queries) {
	t.Helper()
	empty := ""
	if _, err := q.UpdateSettings(ctx, db.UpdateSettingsParams{NewCustomerBlockSet: true, NewCustomerBlockMessage: &empty}); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareBooking_rateLimitedPerIP(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	clearNewCustomerRestriction(t, ctx, q)

	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger())
	srv := New(testConfig(), q, stripetest.New(), emailSvc, nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	day := time.Now().UTC().AddDate(0, 0, 30)
	start := time.Date(day.Year(), day.Month(), day.Day(), 10, 0, 0, 0, time.UTC)
	body, _ := json.Marshal(map[string]any{
		"email":      "card@example.com",
		"start_time": start,
		"end_time":   start.Add(2 * time.Hour),
	})
	prepare := func(i int) int {
		req := mustNewRequest(t, http.MethodPost, ts.URL+"/api/bookings/prepare", bytes.NewReader(body))
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i))
		resp := doReq(t, nil, req)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusTooManyRequests {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("prepare %d: status %d body %s", i, resp.StatusCode, b)
		}
		return resp.StatusCode
	}

	for i := 0; i < bookingPrepareRateLimitMax; i++ {
		if got := prepare(i); got != http.StatusOK {
			t.Fatalf("prepare %d: status %d want 200", i, got)
		}
	}
	if got := prepare(99); got != http.StatusTooManyRequests {
		t.Fatalf("prepare over limit: status %d want 429", got)
	}

	// Invalid requests are rejected before the limit and do not hit Stripe.
	req := mustNewRequest(t, http.MethodPost, ts.URL+"/api/bookings/prepare", strings.NewReader(`{"email":"x@example.com"}`))
	resp := doReq(t, nil, req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid prepare: status %d want 400", resp.StatusCode)
	}
}

// After a charge fails with an unknown outcome, the retry must reuse the idempotency key so
// Stripe replays the first request; after a definite decline, the retry gets a new key.
// Booking creates are rate limited per client IP; idempotent replays of the same
// setup_intent_id do not use up the budget.
func TestCreateBooking_rateLimitedPerIP(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger())
	srv := New(testConfig(), q, stripetest.New(), emailSvc, nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// One payload per distinct day: bookings cannot overlap, so each needs its own slot.
	// Payloads are minted once so a later replay of index 0 reuses its setup_intent_id.
	payloads := make([]string, bookingCreateRateLimitMax+1)
	for i := range payloads {
		start := time.Date(2035, 6, 1+i, 12, 0, 0, 0, time.UTC)
		payloads[i] = fmt.Sprintf(`{"setup_intent_id":%q,"name":"Pat","email":"pat@example.com","start_time":%q,"end_time":%q}`,
			mintSetupIntentID(t), start.Format(time.RFC3339), start.Add(2*time.Hour).Format(time.RFC3339))
	}
	create := func(i int) int {
		req := mustNewRequest(t, http.MethodPost, ts.URL+"/api/bookings", strings.NewReader(payloads[i]))
		resp := doReq(t, ts.Client(), req)
		defer resp.Body.Close()
		return resp.StatusCode
	}

	for i := 0; i < bookingCreateRateLimitMax; i++ {
		if got := create(i); got != http.StatusCreated {
			t.Fatalf("create %d: status %d want 201", i, got)
		}
	}
	if got := create(bookingCreateRateLimitMax); got != http.StatusTooManyRequests {
		t.Fatalf("create over limit: status %d want 429", got)
	}

	// Replaying an earlier setup_intent_id is idempotent and stays under the limit.
	if got := create(0); got != http.StatusCreated {
		t.Fatalf("idempotent replay over limit: status %d want 201", got)
	}
}

func TestAdminChargeBooking_idempotencyKeyAcrossRetries(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateAdminUser(ctx, "admin", string(hash)); err != nil {
		t.Fatal(err)
	}

	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger())
	for i, tc := range []struct {
		name     string
		firstErr error
		sameKey  bool
	}{
		{"network error", errors.New("dial tcp: i/o timeout"), true},
		{"stripe server error", &stripego.Error{Type: stripego.ErrorTypeAPI, HTTPStatusCode: http.StatusInternalServerError}, true},
		{"card declined", &stripego.Error{Type: stripego.ErrorTypeCard, HTTPStatusCode: http.StatusPaymentRequired}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ms := stripetest.NewScripted(tc.firstErr, nil)
			srv := New(testConfig(), q, ms, emailSvc, nil, testLogger())
			ts := httptest.NewServer(srv.Handler())
			t.Cleanup(ts.Close)

			start := time.Date(2031, 3, 1+i, 12, 0, 0, 0, time.UTC)
			b, err := q.CreateBooking(ctx, "Pat", "pat@example.com", nil, start, start.Add(2*time.Hour), "si_"+tc.name, "pm_test_123", nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := q.UpdateBookingStatus(ctx, b.ID, "completed"); err != nil {
				t.Fatal(err)
			}
			truncateLoginAttempts(t, ctx, q)

			sess := loginAndSession(t, ts.URL, "admin", "s3cret!")
			csrfTok := "keycsrf0123456789012345678901ab"
			chargeURL := ts.URL + "/api/admin/bookings/" + strconv.FormatInt(int64(b.ID), 10) + "/charge"
			charge := func() int {
				req := mustNewRequest(t, http.MethodPost, chargeURL, nil)
				addAdminCookies(req, sess, csrfTok)
				req.Header.Set("X-CSRF-Token", csrfTok)
				resp := doReq(t, ts.Client(), req)
				resp.Body.Close()
				return resp.StatusCode
			}

			if got := charge(); got != http.StatusInternalServerError {
				t.Fatalf("first charge: status %d want 500", got)
			}
			if got := charge(); got != http.StatusOK {
				t.Fatalf("retry: status %d want 200", got)
			}
			keys := ms.Keys()
			if len(keys) != 2 {
				t.Fatalf("charge calls = %d want 2", len(keys))
			}
			if same := keys[0] == keys[1]; same != tc.sameKey {
				t.Fatalf("keys %q then %q: same = %v, want %v", keys[0], keys[1], same, tc.sameKey)
			}
		})
	}
}

func truncateLoginAttempts(t *testing.T, ctx context.Context, q *db.Queries) {
	t.Helper()
	if err := q.DeleteRateLimitRecordsBefore(ctx, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
}
