package api

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spencercornish/booklab/internal/db"
)

// testDSN returns a Postgres URL for integration tests. When unset, callers should t.Skip.
func testDSN() string {
	return os.Getenv("TEST_DATABASE_URL")
}

func openTestPool(t *testing.T) (*pgxpool.Pool, *db.Queries) {
	t.Helper()
	dsn := testDSN()
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run integration tests (e.g. postgres://user:pass@localhost:5432/booklab_test)")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool, db.New(pool)
}

func truncateBookingData(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `TRUNCATE login_attempts, admin_sessions, bookings, closures RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
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

// mockStripe implements stripe.Client for tests; counts ChargePaymentMethod calls.
type mockStripe struct {
	mu          sync.Mutex
	chargeCalls int
}

func (m *mockStripe) CreateSetupIntent(email string) (string, string, error) {
	return "si_test_mock", "seti_secret_mock", nil
}

func (m *mockStripe) GetPaymentMethodFromSetupIntent(setupIntentID string) (string, error) {
	return "pm_test_mock", nil
}

func (m *mockStripe) ChargePaymentMethod(pmID string, amountCents int64, currency, description, idempotencyKey string) (string, string, error) {
	m.mu.Lock()
	m.chargeCalls++
	m.mu.Unlock()
	return "pi_test_mock", "https://stripe.test/receipt", nil
}

func (m *mockStripe) DetachPaymentMethod(pmID string) error {
	return nil
}

func (m *mockStripe) chargeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.chargeCalls
}
