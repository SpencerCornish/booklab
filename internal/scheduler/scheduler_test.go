package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spencercornish/booklab/internal/db"
	emailsvc "github.com/spencercornish/booklab/internal/email"
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

type countingStripe struct {
	chargeCalls atomic.Int32
}

func (c *countingStripe) CreateSetupIntent(string) (string, string, error) {
	panic("CreateSetupIntent not used in scheduler tests")
}

func (c *countingStripe) GetPaymentMethodFromSetupIntent(string) (string, error) {
	panic("GetPaymentMethodFromSetupIntent not used in scheduler tests")
}

func (c *countingStripe) ChargePaymentMethod(string, int64, string, string, string) (string, string, error) {
	c.chargeCalls.Add(1)
	return "pi_sched_test", "https://stripe.test/receipt", nil
}

func (c *countingStripe) DetachPaymentMethod(string) error {
	return nil
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
	if _, err := pool.Exec(ctx, `TRUNCATE login_attempts, booking_prepare_attempts, admin_sessions, admin_users, bookings, closures RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	resetSettingsUTC(t, ctx, q)

	start := time.Date(2032, 4, 1, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	b, err := q.CreateBooking(ctx, "Sam", "sam@example.com", nil, start, end, "si_sched", "pm_sched_1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateBookingStatus(ctx, b.ID, db.BookingStatusCompleted); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE bookings SET completed_at = NOW() - INTERVAL '30 hours' WHERE id = $1`, b.ID); err != nil {
		t.Fatal(err)
	}

	st := &countingStripe{}
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

	if got := st.chargeCalls.Load(); got != 1 {
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

// scriptedStripe returns errs from successive charges and records the idempotency keys.
type scriptedStripe struct {
	countingStripe
	mu   sync.Mutex
	errs []error
	keys []string
}

func (c *scriptedStripe) ChargePaymentMethod(_ string, _ int64, _ string, _ string, key string) (string, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keys = append(c.keys, key)
	if len(c.errs) > 0 {
		err := c.errs[0]
		c.errs = c.errs[1:]
		if err != nil {
			return "", "", err
		}
	}
	return "pi_sched_test", "https://stripe.test/receipt", nil
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
	if _, err := pool.Exec(ctx, `TRUNCATE login_attempts, booking_prepare_attempts, admin_sessions, admin_users, bookings, closures RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	resetSettingsUTC(t, ctx, q)

	start := time.Date(2032, 4, 1, 12, 0, 0, 0, time.UTC)
	b, err := q.CreateBooking(ctx, "Sam", "sam@example.com", nil, start, start.Add(2*time.Hour), "si_sched_retry", "pm_sched_2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateBookingStatus(ctx, b.ID, db.BookingStatusCompleted); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE bookings SET completed_at = NOW() - INTERVAL '30 hours' WHERE id = $1`, b.ID); err != nil {
		t.Fatal(err)
	}

	st := &scriptedStripe{errs: []error{errors.New("read tcp: connection reset by peer"), nil}}
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
	if len(st.keys) != 2 || st.keys[0] != st.keys[1] {
		t.Fatalf("idempotency keys %q, want the same key twice", st.keys)
	}
}
