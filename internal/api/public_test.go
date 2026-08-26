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
		SetupIntentID          string `json:"setup_intent_id"`
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
