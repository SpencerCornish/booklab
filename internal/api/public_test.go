package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	emailsvc "github.com/spencercornish/booklab/internal/email"

	"github.com/spencercornish/booklab/internal/db"
)

func TestCreateBooking_requestBodyExceedsLimit(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	ms := &mockStripe{}
	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger())
	srv := New(testConfig(), q, ms, emailSvc, nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	big := strings.Repeat("x", maxJSONBodyBytes+8000)
	payload := fmt.Sprintf(`{"name":"Pat","email":%q,"start_time":"2035-06-10T14:00:00Z","end_time":"2035-06-10T18:00:00Z"}`, big)

	req := mustNewRequest(t, http.MethodPost, ts.URL+"/api/bookings", strings.NewReader(payload))
	resp := doReq(t, ts.Client(), req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d want 413 body %s", resp.StatusCode, b)
	}
}

func TestValidateBookingCreate_rejectsPastStart(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	s := &Server{queries: q, cfg: testConfig(), logger: testLogger()}
	settings, err := q.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(-3 * time.Hour)
	end := start.Add(2 * time.Hour)
	bad, err := s.validateBookingCreate(ctx, start, end, settings)
	if err != nil {
		t.Fatal(err)
	}
	if bad == "" {
		t.Fatal("expected rejection for past start")
	}
}

func TestValidateBookingCreate_rejectsInsideMinLeadTime(t *testing.T) {
	s := &Server{cfg: testConfig(), logger: testLogger()}
	settings := &db.Settings{Timezone: "UTC", MinHours: 1, MaxHours: 8, MinLeadHours: 24}
	start := time.Now().Add(23 * time.Hour)
	bad, err := s.validateBookingCreate(context.Background(), start, start.Add(2*time.Hour), settings)
	if err != nil {
		t.Fatal(err)
	}
	if bad != "bookings must be made at least 24 hours in advance" {
		t.Fatalf("got %q", bad)
	}
}

func TestValidateBookingCreate_allowsBeyondMinLeadTime(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	t.Cleanup(func() {
		zero := int32(0)
		_, _ = q.UpdateSettings(context.Background(), db.UpdateSettingsParams{MinLeadHours: &zero})
	})
	lead := int32(24)
	settings, err := q.UpdateSettings(ctx, db.UpdateSettingsParams{MinLeadHours: &lead})
	if err != nil {
		t.Fatal(err)
	}
	if settings.MinLeadHours != 24 {
		t.Fatalf("min_lead_hours = %d, want 24", settings.MinLeadHours)
	}

	s := &Server{queries: q, cfg: testConfig(), logger: testLogger()}
	start := time.Date(2035, 7, 11, 11, 0, 0, 0, time.UTC)
	bad, err := s.validateBookingCreate(ctx, start, start.Add(2*time.Hour), settings)
	if err != nil {
		t.Fatal(err)
	}
	if bad != "" {
		t.Fatalf("expected booking well beyond the notice period to pass, got %q", bad)
	}
}

func TestValidateBookingCreate_rejectsOutsideBookableHours(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	s := &Server{queries: q, cfg: testConfig(), logger: testLogger()}
	settings, err := q.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Window 10:00–16:00 UTC; 08:00–10:00 is two hours (meets min) but starts before window.
	start := time.Date(2035, 7, 10, 8, 0, 0, 0, time.UTC)
	end := time.Date(2035, 7, 10, 10, 0, 0, 0, time.UTC)
	bad, err := s.validateBookingCreate(ctx, start, end, settings)
	if err != nil {
		t.Fatal(err)
	}
	if bad == "" {
		t.Fatal("expected rejection for out-of-window booking")
	}
}

func TestValidateBookingCreate_rejectsUnderMinDuration(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	s := &Server{queries: q, cfg: testConfig(), logger: testLogger()}
	settings, err := q.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2035, 7, 11, 11, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour) // 1 hour < min_hours 2
	bad, err := s.validateBookingCreate(ctx, start, end, settings)
	if err != nil {
		t.Fatal(err)
	}
	if bad == "" {
		t.Fatal("expected rejection for short booking")
	}
}

func TestValidateBookingCreate_rejectsOverMaxDuration(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	orig, err := q.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		mh := orig.MaxHours
		_, _ = q.UpdateSettings(context.Background(), db.UpdateSettingsParams{MaxHours: &mh})
	})

	mh := int32(2)
	if _, err := q.UpdateSettings(ctx, db.UpdateSettingsParams{MaxHours: &mh}); err != nil {
		t.Fatal(err)
	}
	settings, err := q.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{queries: q, cfg: testConfig(), logger: testLogger()}
	start := time.Date(2035, 7, 12, 11, 0, 0, 0, time.UTC)
	end := start.Add(4 * time.Hour)
	bad, err := s.validateBookingCreate(ctx, start, end, settings)
	if err != nil {
		t.Fatal(err)
	}
	if bad == "" {
		t.Fatal("expected rejection for booking longer than max_hours")
	}
}

func TestValidateBookingCreate_rejectsClosureOverlap(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	startC := time.Date(2035, 8, 1, 0, 0, 0, 0, time.UTC)
	endC := time.Date(2035, 8, 31, 0, 0, 0, 0, time.UTC)
	if _, err := q.CreateClosure(ctx, startC, endC, true, nil, nil, nil); err != nil {
		t.Fatal(err)
	}

	s := &Server{queries: q, cfg: testConfig(), logger: testLogger()}
	settings, err := q.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2035, 8, 15, 11, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	bad, err := s.validateBookingCreate(ctx, start, end, settings)
	if err != nil {
		t.Fatal(err)
	}
	if bad == "" {
		t.Fatal("expected rejection when booking overlaps a closure")
	}
}

func TestPrepareBooking_returnsSetupIntent(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	ms := &mockStripe{}
	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger())
	srv := New(testConfig(), q, ms, emailSvc, nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	start := time.Date(2035, 6, 10, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	payload := fmt.Sprintf(`{"email":"pat@example.com","start_time":%q,"end_time":%q}`, start.Format(time.RFC3339), end.Format(time.RFC3339))

	req := mustNewRequest(t, http.MethodPost, ts.URL+"/api/bookings/prepare", strings.NewReader(payload))
	resp := doReq(t, ts.Client(), req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d want 200 body %s", resp.StatusCode, b)
	}
	var body struct {
		SetupIntentID           string `json:"setup_intent_id"`
		SetupIntentClientSecret string `json:"setup_intent_client_secret"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.SetupIntentID == "" || body.SetupIntentClientSecret == "" {
		t.Fatalf("missing setup intent fields: %+v", body)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM bookings`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("prepare created booking rows: count=%d", count)
	}
}

func TestCreateBooking_finalizeInsertsWithPaymentMethod(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	ms := &mockStripe{}
	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger())
	srv := New(testConfig(), q, ms, emailSvc, nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	start := time.Date(2035, 6, 11, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	payload := fmt.Sprintf(`{"setup_intent_id":"si_test_mock","name":"Pat","email":"pat@example.com","start_time":%q,"end_time":%q}`,
		start.Format(time.RFC3339), end.Format(time.RFC3339))

	req := mustNewRequest(t, http.MethodPost, ts.URL+"/api/bookings", strings.NewReader(payload))
	resp := doReq(t, ts.Client(), req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d want 201 body %s", resp.StatusCode, b)
	}

	b, err := q.ListBookings(ctx, db.ListBookingsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 1 {
		t.Fatalf("bookings count = %d, want 1", len(b))
	}
	if b[0].StripePaymentMethodID == nil || *b[0].StripePaymentMethodID != "pm_test_mock" {
		t.Fatalf("payment method = %v, want pm_test_mock", b[0].StripePaymentMethodID)
	}
}

func TestCreateBooking_idempotentBySetupIntent(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)

	ms := &mockStripe{}
	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger())
	srv := New(testConfig(), q, ms, emailSvc, nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	start := time.Date(2035, 6, 12, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	payload := fmt.Sprintf(`{"setup_intent_id":"si_test_mock","name":"Pat","email":"pat@example.com","start_time":%q,"end_time":%q}`,
		start.Format(time.RFC3339), end.Format(time.RFC3339))

	for i := 0; i < 2; i++ {
		req := mustNewRequest(t, http.MethodPost, ts.URL+"/api/bookings", strings.NewReader(payload))
		resp := doReq(t, ts.Client(), req)
		if resp.StatusCode != http.StatusCreated {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("attempt %d: status %d body %s", i+1, resp.StatusCode, b)
		}
		resp.Body.Close()
	}

	b, err := q.ListBookings(ctx, db.ListBookingsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 1 {
		t.Fatalf("bookings count = %d, want 1", len(b))
	}
}

// newCustomerTestServer starts the API with a mock Stripe client for restriction tests.
func newCustomerTestServer(t *testing.T, q *db.Queries) *httptest.Server {
	t.Helper()
	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger())
	srv := New(testConfig(), q, &mockStripe{}, emailSvc, nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// setNewCustomerWindow stores the window and message. Nil dates clear the window.
func setNewCustomerWindow(t *testing.T, ctx context.Context, q *db.Queries, start, end *time.Time, message string) {
	t.Helper()
	if _, err := q.UpdateSettings(ctx, db.UpdateSettingsParams{
		NewCustomerBlockSet:     true,
		NewCustomerBlockStart:   start,
		NewCustomerBlockEnd:     end,
		NewCustomerBlockMessage: &message,
	}); err != nil {
		t.Fatalf("set new customer window: %v", err)
	}
}

func dateAt(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

func postJSONForTest(t *testing.T, ts *httptest.Server, path, payload string) (int, map[string]any) {
	t.Helper()
	req := mustNewRequest(t, http.MethodPost, ts.URL+path, strings.NewReader(payload))
	resp := doReq(t, ts.Client(), req)
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

func prepareBookingPayload(email string, start, end time.Time) string {
	return fmt.Sprintf(`{"email":%q,"start_time":%q,"end_time":%q}`, email, start.Format(time.RFC3339), end.Format(time.RFC3339))
}

func createBookingPayload(start, end time.Time) string {
	return fmt.Sprintf(`{"setup_intent_id":"si_test_mock","name":"Pat","email":"pat@example.com","start_time":%q,"end_time":%q}`,
		start.Format(time.RFC3339), end.Format(time.RFC3339))
}

func TestNewCustomerRestriction_blocksNewCustomerInsideWindow(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	setNewCustomerWindow(t, ctx, q, dateAt(2035, 6, 1), dateAt(2035, 6, 30), "Closed for vacation")
	ts := newCustomerTestServer(t, q)

	start := time.Date(2035, 6, 10, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)

	status, body := postJSONForTest(t, ts, "/api/bookings/prepare", prepareBookingPayload("pat@example.com", start, end))
	if status != http.StatusForbidden || body["error"] != "Closed for vacation" {
		t.Fatalf("prepare: status %d body %v, want 403 with restriction message", status, body)
	}

	status, body = postJSONForTest(t, ts, "/api/bookings", createBookingPayload(start, end))
	if status != http.StatusForbidden || body["error"] != "Closed for vacation" {
		t.Fatalf("create: status %d body %v, want 403 with restriction message", status, body)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM bookings`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("blocked create inserted %d booking rows", count)
	}
}

func TestNewCustomerRestriction_defaultMessageWhenBlank(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	setNewCustomerWindow(t, ctx, q, dateAt(2035, 6, 1), dateAt(2035, 6, 30), "")
	ts := newCustomerTestServer(t, q)

	start := time.Date(2035, 6, 10, 12, 0, 0, 0, time.UTC)
	status, body := postJSONForTest(t, ts, "/api/bookings/prepare", prepareBookingPayload("pat@example.com", start, start.Add(2*time.Hour)))
	want := "We're only accepting returning customers between June 1, 2035 and June 30, 2035."
	if status != http.StatusForbidden || body["error"] != want {
		t.Fatalf("status %d body %v, want 403 with %q", status, body, want)
	}
}

func TestNewCustomerRestriction_confirmedFutureBookingDoesNotCount(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	setNewCustomerWindow(t, ctx, q, dateAt(2035, 6, 1), dateAt(2035, 6, 30), "Closed")
	ts := newCustomerTestServer(t, q)

	// A confirmed booking on a far-off date must not unlock the window.
	far := time.Date(2035, 8, 1, 12, 0, 0, 0, time.UTC)
	if _, err := q.CreateBooking(ctx, "Pat", "pat@example.com", nil, far, far.Add(2*time.Hour), "si_future", "pm_test_123"); err != nil {
		t.Fatal(err)
	}

	start := time.Date(2035, 6, 10, 12, 0, 0, 0, time.UTC)
	status, _ := postJSONForTest(t, ts, "/api/bookings/prepare", prepareBookingPayload("pat@example.com", start, start.Add(2*time.Hour)))
	if status != http.StatusForbidden {
		t.Fatalf("status %d, want 403 for customer with only a confirmed booking", status)
	}
}

func TestNewCustomerRestriction_returningCustomerAllowed(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	setNewCustomerWindow(t, ctx, q, dateAt(2035, 6, 1), dateAt(2035, 6, 30), "Closed")
	ts := newCustomerTestServer(t, q)

	// Email match ignores case and surrounding whitespace; a completed booking in the past unlocks the window.
	past := time.Date(2031, 3, 1, 12, 0, 0, 0, time.UTC)
	b, err := q.CreateBooking(ctx, "Pat", " Pat@Example.com ", nil, past, past.Add(2*time.Hour), "si_past", "pm_test_123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateBookingStatus(ctx, b.ID, db.BookingStatus("completed")); err != nil {
		t.Fatal(err)
	}

	start := time.Date(2035, 6, 10, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	status, body := postJSONForTest(t, ts, "/api/bookings/prepare", prepareBookingPayload("pat@example.com", start, end))
	if status != http.StatusOK {
		t.Fatalf("prepare: status %d body %v, want 200 for returning customer", status, body)
	}
	status, body = postJSONForTest(t, ts, "/api/bookings", createBookingPayload(start, end))
	if status != http.StatusCreated {
		t.Fatalf("create: status %d body %v, want 201 for returning customer", status, body)
	}
}

func TestNewCustomerRestriction_historyStatusesCountAsReturning(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	setNewCustomerWindow(t, ctx, q, dateAt(2035, 6, 1), dateAt(2035, 6, 30), "Closed")
	ts := newCustomerTestServer(t, q)

	start := time.Date(2035, 6, 10, 12, 0, 0, 0, time.UTC)
	for i, st := range []db.BookingStatus{"charging", "charged"} {
		past := time.Date(2031, 3, 1+i, 12, 0, 0, 0, time.UTC)
		email := fmt.Sprintf("returning%d@example.com", i)
		b, err := q.CreateBooking(ctx, "Pat", email, nil, past, past.Add(2*time.Hour), fmt.Sprintf("si_hist_%d", i), "pm_test_123")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.UpdateBookingStatus(ctx, b.ID, st); err != nil {
			t.Fatal(err)
		}
		status, body := postJSONForTest(t, ts, "/api/bookings/prepare", prepareBookingPayload(email, start, start.Add(2*time.Hour)))
		if status != http.StatusOK {
			t.Fatalf("status %s: prepare %d body %v, want 200", st, status, body)
		}
	}
}

func TestNewCustomerRestriction_allowsNewCustomerOutsideWindow(t *testing.T) {
	cases := []struct {
		name         string
		start, end   *time.Time
		sessionStart time.Time
	}{
		{"session after window", dateAt(2035, 6, 1), dateAt(2035, 6, 30), time.Date(2035, 7, 10, 12, 0, 0, 0, time.UTC)},
		{"session before window", dateAt(2035, 6, 1), dateAt(2035, 6, 30), time.Date(2035, 5, 10, 12, 0, 0, 0, time.UTC)},
		{"window cleared", nil, nil, time.Date(2035, 6, 10, 12, 0, 0, 0, time.UTC)},
		{"window ended", dateAt(2020, 1, 1), dateAt(2020, 1, 31), time.Date(2035, 6, 10, 12, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			pool, q := openTestPool(t)
			truncateBookingData(t, ctx, pool)
			resetSettingsUTC(t, ctx, q)
			setNewCustomerWindow(t, ctx, q, tc.start, tc.end, "Closed")
			ts := newCustomerTestServer(t, q)

			status, body := postJSONForTest(t, ts, "/api/bookings/prepare",
				prepareBookingPayload("pat@example.com", tc.sessionStart, tc.sessionStart.Add(2*time.Hour)))
			if status != http.StatusOK {
				t.Fatalf("status %d body %v, want 200", status, body)
			}
		})
	}
}

// The window is whole days in the settings timezone, so the edges fall at local midnight.
// Denver is UTC-6 in June: the window is 2035-06-10 06:00Z to 2035-06-11 06:00Z.
func TestNewCustomerRestriction_usesSettingsTimezone(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	denver := "America/Denver"
	bs := time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC)
	be := time.Date(0, 1, 1, 23, 0, 0, 0, time.UTC)
	if _, err := q.UpdateSettings(ctx, db.UpdateSettingsParams{Timezone: &denver, BookableStart: &bs, BookableEnd: &be}); err != nil {
		t.Fatal(err)
	}
	setNewCustomerWindow(t, ctx, q, dateAt(2035, 6, 10), dateAt(2035, 6, 10), "Denver closed")
	ts := newCustomerTestServer(t, q)

	cases := []struct {
		name       string
		start, end time.Time
		wantStatus int
	}{
		// Denver 19:00-21:00 on June 10 is 01:00-03:00Z on June 11: inside the local window, outside the UTC one.
		{"inside local window", time.Date(2035, 6, 11, 1, 0, 0, 0, time.UTC), time.Date(2035, 6, 11, 3, 0, 0, 0, time.UTC), http.StatusForbidden},
		// Denver 21:00-23:00 on June 9 ends at 05:00Z on June 10, before the window opens.
		{"ends before window opens", time.Date(2035, 6, 10, 3, 0, 0, 0, time.UTC), time.Date(2035, 6, 10, 5, 0, 0, 0, time.UTC), http.StatusOK},
		// Denver 00:00 on June 11 is the exclusive window end.
		{"starts at window end", time.Date(2035, 6, 11, 6, 0, 0, 0, time.UTC), time.Date(2035, 6, 11, 8, 0, 0, 0, time.UTC), http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := postJSONForTest(t, ts, "/api/bookings/prepare", prepareBookingPayload("pat@example.com", tc.start, tc.end))
			if status != tc.wantStatus {
				t.Fatalf("status %d body %v, want %d", status, body, tc.wantStatus)
			}
		})
	}
}

func getJSONForTest(t *testing.T, ts *httptest.Server, path string) map[string]any {
	t.Helper()
	req := mustNewRequest(t, http.MethodGet, ts.URL+path, nil)
	resp := doReq(t, ts.Client(), req)
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestGetPublicSettings_newCustomerRestriction(t *testing.T) {
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	ts := newCustomerTestServer(t, q)

	t.Run("active window with message", func(t *testing.T) {
		setNewCustomerWindow(t, ctx, q, dateAt(2035, 6, 1), dateAt(2035, 6, 30), "Closed for vacation")
		got, _ := getJSONForTest(t, ts, "/api/settings/public")["new_customer_restriction"].(map[string]any)
		if got == nil || got["start_date"] != "2035-06-01" || got["end_date"] != "2035-06-30" || got["message"] != "Closed for vacation" {
			t.Fatalf("restriction = %v", got)
		}
	})

	t.Run("active window with blank message gets default", func(t *testing.T) {
		setNewCustomerWindow(t, ctx, q, dateAt(2035, 6, 1), dateAt(2035, 6, 30), "")
		got, _ := getJSONForTest(t, ts, "/api/settings/public")["new_customer_restriction"].(map[string]any)
		if got == nil || got["message"] != "We're only accepting returning customers between June 1, 2035 and June 30, 2035." {
			t.Fatalf("restriction = %v", got)
		}
	})

	t.Run("ended window is hidden", func(t *testing.T) {
		setNewCustomerWindow(t, ctx, q, dateAt(2020, 1, 1), dateAt(2020, 1, 31), "Old")
		body := getJSONForTest(t, ts, "/api/settings/public")
		if v, ok := body["new_customer_restriction"]; !ok || v != nil {
			t.Fatalf("new_customer_restriction = %v, want null", v)
		}
	})

	t.Run("cleared window is hidden", func(t *testing.T) {
		setNewCustomerWindow(t, ctx, q, nil, nil, "")
		body := getJSONForTest(t, ts, "/api/settings/public")
		if v, ok := body["new_customer_restriction"]; !ok || v != nil {
			t.Fatalf("new_customer_restriction = %v, want null", v)
		}
	})
}
