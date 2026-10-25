package api

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spencercornish/booklab/internal/db"
)

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

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
	_, err := pool.Exec(ctx, `TRUNCATE login_attempts, booking_prepare_attempts, booking_create_attempts, admin_sessions, admin_users, bookings, closures RESTART IDENTITY CASCADE`)
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
	gateEnabled := false
	gateSchedule := []db.NewUserWindow{}
	gateQuestion := "Have you used this space before?"
	_, err := q.UpdateSettings(ctx, db.UpdateSettingsParams{
		Timezone:            &tz,
		BookableStart:       &bs,
		BookableEnd:         &be,
		MinHours:            &minH,
		MaxHours:            &maxH,
		HourlyRateCents:     &hr,
		Currency:            &cur,
		NewUserGateEnabled:  &gateEnabled,
		NewUserGateQuestion: &gateQuestion,
		NewUserGateSchedule: &gateSchedule,
	})
	if err != nil {
		t.Fatalf("reset settings: %v", err)
	}
}

// The Stripe test double lives in internal/stripe/stripetest and is shared by
// every suite (api, scheduler, and the functional tests via STRIPE_FAKE=1).
