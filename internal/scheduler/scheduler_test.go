package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/spencercornish/booklab/internal/db"
	emailsvc "github.com/spencercornish/booklab/internal/email"
	"github.com/spencercornish/booklab/internal/stripe/stripetest"
)

func testDSN() string {
	return os.Getenv("TEST_DATABASE_URL")
}

func resetSettingsUTC(t *testing.T, ctx context.Context, q *db.Queries) {
	t.Helper()
	tz := "UTC"
	bs := time.Date(0, 1, 1, 10, 0, 0, 0, time.UTC)
	be := time.Date(0, 1, 1, 16, 0, 0, 0, time.UTC)
	minH := int32(2)
	maxH := int32(8)
	hr := int32(1000)
	cur := "usd"
	_, err := q.UpdateSettings(ctx, db.UpdateSettingsParams{
		Timezone:        &tz,
		BookableStart:   &bs,
		BookableEnd:     &be,
		MinHours:        &minH,
		MaxHours:        &maxH,
		HourlyRateCents: &hr,
		Currency:        &cur,
	})
	if err != nil {
		t.Fatalf("reset settings: %v", err)
	}
}

func TestAutoCharge_completed_concurrentOnlyOneCharge(t *testing.T) {
	dsn := testDSN()
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run integration tests")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })

	q := db.New(pool)
	if _, err := pool.Exec(ctx, `TRUNCATE login_attempts, booking_prepare_attempts, booking_create_attempts, admin_sessions, admin_users, bookings, closures RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	resetSettingsUTC(t, ctx, q)

	start := time.Date(2032, 4, 1, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	b, err := q.CreateBooking(ctx, "Sam", "sam@example.com", nil, start, end, "si_sched", "pm_sched_1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateBookingStatus(ctx, b.ID, db.BookingStatusCompleted); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE bookings SET completed_at = NOW() - INTERVAL '30 hours' WHERE id = $1`, b.ID); err != nil {
		t.Fatal(err)
	}

	st := stripetest.NewCounting()
	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "sched@localhost", slog.New(slog.DiscardHandler))
	s := New(q, emailSvc, st, "http://127.0.0.1:8080", slog.New(slog.DiscardHandler))

	settings, err := q.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.autoChargeCompleted(ctx, settings)
		}()
	}
	wg.Wait()

	if got := st.ChargeCalls(); got != 1 {
		t.Fatalf("ChargePaymentMethod calls = %d, want 1", got)
	}
	nb, err := q.GetBookingByID(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nb.Status != db.BookingStatusCharged {
		t.Fatalf("booking status = %s, want charged", nb.Status)
	}
}

// An auto-charge whose outcome is unknown must be retried with the same idempotency key,
// so a charge that actually went through is replayed by Stripe rather than taken twice.
func TestAutoCharge_unknownOutcomeRetryReusesKey(t *testing.T) {
	dsn := testDSN()
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run integration tests")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })

	q := db.New(pool)
	if _, err := pool.Exec(ctx, `TRUNCATE login_attempts, booking_prepare_attempts, booking_create_attempts, admin_sessions, admin_users, bookings, closures RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	resetSettingsUTC(t, ctx, q)

	start := time.Date(2032, 4, 1, 12, 0, 0, 0, time.UTC)
	b, err := q.CreateBooking(ctx, "Sam", "sam@example.com", nil, start, start.Add(2*time.Hour), "si_sched_retry", "pm_sched_2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateBookingStatus(ctx, b.ID, db.BookingStatusCompleted); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE bookings SET completed_at = NOW() - INTERVAL '30 hours' WHERE id = $1`, b.ID); err != nil {
		t.Fatal(err)
	}

	st := stripetest.NewScripted(errors.New("read tcp: connection reset by peer"), nil)
	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "sched@localhost", slog.New(slog.DiscardHandler))
	s := New(q, emailSvc, st, "http://127.0.0.1:8080", slog.New(slog.DiscardHandler))
	settings, err := q.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// The failed attempt puts the booking back and the same run claims it again.
	if got := s.autoChargeCompleted(ctx, settings); got != 1 {
		t.Fatalf("charged %d, want 1", got)
	}
	if keys := st.Keys(); len(keys) != 2 || keys[0] != keys[1] {
		t.Fatalf("idempotency keys %q, want the same key twice", keys)
	}
}
