package scheduler

import (
	"context"
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
	if _, err := pool.Exec(ctx, `TRUNCATE login_attempts, admin_sessions, bookings, closures RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	resetSettingsUTC(t, ctx, q)

	start := time.Date(2032, 4, 1, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	b, err := q.CreateBooking(ctx, "Sam", "sam@example.com", start, end, "si_sched")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateBookingPaymentMethod(ctx, b.ID, "pm_sched_1"); err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpdateBookingStatus(ctx, b.ID, db.BookingStatusCompleted); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE bookings SET completed_at = NOW() - INTERVAL '30 hours' WHERE id = $1`, b.ID); err != nil {
		t.Fatal(err)
	}

	st := &countingStripe{}
	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "sched@localhost")
	s := New(q, emailSvc, st, "http://127.0.0.1:8080")

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
