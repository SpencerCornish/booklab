package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Queries struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Queries {
	return &Queries{pool: pool}
}

// ----- Settings -----

func (q *Queries) GetSettings(ctx context.Context) (*Settings, error) {
	row := q.pool.QueryRow(ctx, `SELECT id, resource_name, hourly_rate_cents, currency, timezone,
		bookable_start, bookable_end, min_hours, max_hours, reminder_hours_before, notification_emails
		FROM settings WHERE id = 1`)
	return scanSettings(row)
}

type UpdateSettingsParams struct {
	ResourceName        *string
	HourlyRateCents     *int32
	Currency            *string
	Timezone            *string
	BookableStart       *time.Time
	BookableEnd         *time.Time
	MinHours            *int32
	MaxHours            *int32
	ReminderHoursBefore *int32
	NotificationEmails  *string
}

func (q *Queries) UpdateSettings(ctx context.Context, p UpdateSettingsParams) (*Settings, error) {
	row := q.pool.QueryRow(ctx, `
		UPDATE settings SET
			resource_name         = COALESCE($1, resource_name),
			hourly_rate_cents     = COALESCE($2, hourly_rate_cents),
			currency              = COALESCE($3, currency),
			timezone              = COALESCE($4, timezone),
			bookable_start        = COALESCE($5, bookable_start),
			bookable_end          = COALESCE($6, bookable_end),
			min_hours             = COALESCE($7, min_hours),
			max_hours             = COALESCE($8, max_hours),
			reminder_hours_before = COALESCE($9, reminder_hours_before),
			notification_emails   = COALESCE($10, notification_emails)
		WHERE id = 1
		RETURNING id, resource_name, hourly_rate_cents, currency, timezone,
			bookable_start, bookable_end, min_hours, max_hours, reminder_hours_before,
			notification_emails`,
		p.ResourceName, p.HourlyRateCents, p.Currency, p.Timezone,
		p.BookableStart, p.BookableEnd, p.MinHours, p.MaxHours, p.ReminderHoursBefore,
		p.NotificationEmails,
	)
	return scanSettings(row)
}

func scanSettings(row pgx.Row) (*Settings, error) {
	var s Settings
	err := row.Scan(
		&s.ID, &s.ResourceName, &s.HourlyRateCents, &s.Currency, &s.Timezone,
		&s.BookableStart, &s.BookableEnd, &s.MinHours, &s.MaxHours, &s.ReminderHoursBefore,
		&s.NotificationEmails,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ----- Bookings -----

func (q *Queries) CreateBooking(ctx context.Context, name, email string, start, end time.Time, setupIntentID string) (*Booking, error) {
	row := q.pool.QueryRow(ctx, `
		INSERT INTO bookings (name, email, start_time, end_time, stripe_setup_intent_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+bookingColumns,
		name, email, start, end, setupIntentID,
	)
	return scanBooking(row)
}

func (q *Queries) GetBookingByID(ctx context.Context, id int32) (*Booking, error) {
	row := q.pool.QueryRow(ctx, `SELECT `+bookingColumns+` FROM bookings WHERE id = $1`, id)
	return scanBooking(row)
}

func (q *Queries) GetBookingByToken(ctx context.Context, token uuid.UUID) (*Booking, error) {
	row := q.pool.QueryRow(ctx, `SELECT `+bookingColumns+` FROM bookings WHERE cancel_token = $1`, token)
	return scanBooking(row)
}

type ListBookingsParams struct {
	Date   *time.Time
	Status *BookingStatus
}

func (q *Queries) ListBookings(ctx context.Context, p ListBookingsParams) ([]*Booking, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT `+bookingColumns+` FROM bookings
		WHERE ($1::date IS NULL OR start_time::date = $1)
		  AND ($2::text IS NULL OR status = $2::booking_status)
		ORDER BY start_time DESC`,
		p.Date, p.Status,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectBookings(rows)
}

func (q *Queries) ListBookingsInRange(ctx context.Context, from, to time.Time) ([]*Booking, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT `+bookingColumns+` FROM bookings
		WHERE start_time >= $1 AND start_time < $2 AND status != 'cancelled'
		ORDER BY start_time`,
		from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectBookings(rows)
}

func (q *Queries) ListBookingsDueReminder(ctx context.Context, withinHours int) ([]*Booking, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT `+bookingColumns+` FROM bookings
		WHERE reminder_sent = FALSE AND status = 'confirmed'
		  AND start_time BETWEEN NOW() AND NOW() + ($1 || ' hours')::interval`,
		fmt.Sprintf("%d", withinHours),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectBookings(rows)
}

func (q *Queries) UpdateBookingStatus(ctx context.Context, id int32, status BookingStatus) (*Booking, error) {
	row := q.pool.QueryRow(ctx,
		`UPDATE bookings SET status = $2,
			completed_at = CASE WHEN $2::booking_status = 'completed' THEN NOW() ELSE completed_at END
		WHERE id = $1 RETURNING `+bookingColumns,
		id, status,
	)
	return scanBooking(row)
}

func (q *Queries) UpdateBookingEndTime(ctx context.Context, id int32, endTime time.Time) (*Booking, error) {
	row := q.pool.QueryRow(ctx,
		`UPDATE bookings SET end_time = $2 WHERE id = $1 RETURNING `+bookingColumns,
		id, endTime,
	)
	return scanBooking(row)
}

func (q *Queries) UpdateBookingPaymentMethod(ctx context.Context, id int32, pmID string) (*Booking, error) {
	row := q.pool.QueryRow(ctx,
		`UPDATE bookings SET stripe_payment_method_id = $2 WHERE id = $1 RETURNING `+bookingColumns,
		id, pmID,
	)
	return scanBooking(row)
}

func (q *Queries) UpdateBookingCharged(ctx context.Context, id int32, paymentIntentID, receiptURL string, amountCents int32) (*Booking, error) {
	var receiptPtr *string
	if receiptURL != "" {
		receiptPtr = &receiptURL
	}
	row := q.pool.QueryRow(ctx, `
		UPDATE bookings SET status = 'charged', stripe_payment_intent_id = $2,
			stripe_receipt_url = COALESCE($3, stripe_receipt_url), amount_cents = $4
		WHERE id = $1 AND status = 'charging' RETURNING `+bookingColumns,
		id, paymentIntentID, receiptPtr, amountCents,
	)
	return scanBooking(row)
}

// ClaimBookingForCharge atomically marks a completed, unpaid booking as charging.
// Returns pgx.ErrNoRows if the booking is missing or not eligible.
func (q *Queries) ClaimBookingForCharge(ctx context.Context, id int32) (*Booking, error) {
	row := q.pool.QueryRow(ctx, `
		UPDATE bookings SET status = 'charging'
		WHERE id = $1
		  AND status = 'completed'
		  AND stripe_payment_method_id IS NOT NULL
		  AND stripe_payment_intent_id IS NULL
		  AND amount_cents IS NULL
		RETURNING `+bookingColumns,
		id,
	)
	return scanBooking(row)
}

// ClaimBookingForAutoCharge claims the next eligible booking for auto-charge (at most one row).
// Uses FOR UPDATE SKIP LOCKED so concurrent schedulers do not select the same booking.
func (q *Queries) ClaimBookingForAutoCharge(ctx context.Context) (*Booking, error) {
	row := q.pool.QueryRow(ctx, `
		WITH pick AS (
			SELECT id FROM bookings
			WHERE status = 'completed'
			  AND completed_at IS NOT NULL
			  AND completed_at < NOW() - INTERVAL '24 hours'
			  AND stripe_payment_method_id IS NOT NULL
			  AND stripe_payment_intent_id IS NULL
			  AND amount_cents IS NULL
			ORDER BY completed_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE bookings AS b
		SET status = 'charging'
		FROM pick
		WHERE b.id = pick.id
		RETURNING `+bookingColumns)
	return scanBooking(row)
}

// RevertBookingFromChargingToCompleted undoes a charge claim after a failed payment attempt.
func (q *Queries) RevertBookingFromChargingToCompleted(ctx context.Context, id int32) error {
	_, err := q.pool.Exec(ctx, `
		UPDATE bookings SET status = 'completed'
		WHERE id = $1 AND status = 'charging'`,
		id,
	)
	return err
}

func (q *Queries) ListBookingsDueAutoCharge(ctx context.Context) ([]*Booking, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT `+bookingColumns+` FROM bookings
		WHERE status = 'completed'
		  AND completed_at IS NOT NULL
		  AND completed_at < NOW() - INTERVAL '24 hours'
		  AND stripe_payment_method_id IS NOT NULL
		  AND stripe_payment_intent_id IS NULL
		  AND amount_cents IS NULL
		ORDER BY completed_at`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectBookings(rows)
}

func (q *Queries) CountPriorBookings(ctx context.Context, email string, excludeID int32) (int64, error) {
	var count int64
	err := q.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM bookings WHERE email = $1 AND id != $2 AND status != 'cancelled'`,
		email, excludeID,
	).Scan(&count)
	return count, err
}

func (q *Queries) MarkReminderSent(ctx context.Context, id int32) error {
	_, err := q.pool.Exec(ctx, `UPDATE bookings SET reminder_sent = TRUE WHERE id = $1`, id)
	return err
}

func (q *Queries) CancelBooking(ctx context.Context, token uuid.UUID) (*Booking, error) {
	row := q.pool.QueryRow(ctx, `
		UPDATE bookings SET status = 'cancelled'
		WHERE cancel_token = $1 AND status = 'confirmed'
		RETURNING `+bookingColumns,
		token,
	)
	return scanBooking(row)
}

const bookingColumns = `id, name, email, start_time, end_time, status, cancel_token,
	stripe_setup_intent_id, stripe_payment_method_id, stripe_payment_intent_id,
	stripe_receipt_url, amount_cents, reminder_sent, completed_at, created_at, updated_at`

func scanBooking(row pgx.Row) (*Booking, error) {
	var b Booking
	err := row.Scan(
		&b.ID, &b.Name, &b.Email, &b.StartTime, &b.EndTime, &b.Status, &b.CancelToken,
		&b.StripeSetupIntentID, &b.StripePaymentMethodID, &b.StripePaymentIntentID,
		&b.StripeReceiptURL, &b.AmountCents, &b.ReminderSent, &b.CompletedAt,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func collectBookings(rows pgx.Rows) ([]*Booking, error) {
	var bookings []*Booking
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		bookings = append(bookings, b)
	}
	return bookings, rows.Err()
}

// ----- Closures -----

func (q *Queries) ListClosures(ctx context.Context) ([]*Closure, error) {
	rows, err := q.pool.Query(ctx, `SELECT id, start_date, end_date, reason, created_at FROM closures ORDER BY start_date`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectClosures(rows)
}

func (q *Queries) ListClosuresInRange(ctx context.Context, from, to time.Time) ([]*Closure, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT id, start_date, end_date, reason, created_at FROM closures
		WHERE start_date <= $2 AND end_date >= $1
		ORDER BY start_date`,
		from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectClosures(rows)
}

func (q *Queries) GetClosure(ctx context.Context, id int32) (*Closure, error) {
	row := q.pool.QueryRow(ctx, `SELECT id, start_date, end_date, reason, created_at FROM closures WHERE id = $1`, id)
	return scanClosure(row)
}

func (q *Queries) CreateClosure(ctx context.Context, start, end time.Time, reason *string) (*Closure, error) {
	row := q.pool.QueryRow(ctx, `
		INSERT INTO closures (start_date, end_date, reason) VALUES ($1, $2, $3)
		RETURNING id, start_date, end_date, reason, created_at`,
		start, end, reason,
	)
	return scanClosure(row)
}

func (q *Queries) UpdateClosure(ctx context.Context, id int32, start, end time.Time, reason *string) (*Closure, error) {
	row := q.pool.QueryRow(ctx, `
		UPDATE closures SET start_date = $2, end_date = $3, reason = $4
		WHERE id = $1
		RETURNING id, start_date, end_date, reason, created_at`,
		id, start, end, reason,
	)
	return scanClosure(row)
}

func (q *Queries) DeleteClosure(ctx context.Context, id int32) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM closures WHERE id = $1`, id)
	return err
}

func scanClosure(row pgx.Row) (*Closure, error) {
	var c Closure
	err := row.Scan(&c.ID, &c.StartDate, &c.EndDate, &c.Reason, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func collectClosures(rows pgx.Rows) ([]*Closure, error) {
	var closures []*Closure
	for rows.Next() {
		c, err := scanClosure(rows)
		if err != nil {
			return nil, err
		}
		closures = append(closures, c)
	}
	return closures, rows.Err()
}

// ----- Admin Users -----

func (q *Queries) GetAdminByUsername(ctx context.Context, username string) (*AdminUser, error) {
	row := q.pool.QueryRow(ctx, `SELECT id, username, password_hash, created_at FROM admin_users WHERE username = $1`, username)
	var u AdminUser
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (q *Queries) CreateAdminUser(ctx context.Context, username, passwordHash string) (*AdminUser, error) {
	row := q.pool.QueryRow(ctx, `
		INSERT INTO admin_users (username, password_hash) VALUES ($1, $2)
		RETURNING id, username, password_hash, created_at`,
		username, passwordHash,
	)
	var u AdminUser
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// ----- Admin sessions -----

func (q *Queries) CreateAdminSession(ctx context.Context, id, username string, expiresAt time.Time) error {
	_, err := q.pool.Exec(ctx, `
		INSERT INTO admin_sessions (id, username, expires_at) VALUES ($1, $2, $3)`,
		id, username, expiresAt,
	)
	return err
}

func (q *Queries) GetAdminSession(ctx context.Context, id string) (*AdminSession, error) {
	row := q.pool.QueryRow(ctx, `
		SELECT id, username, expires_at, created_at
		FROM admin_sessions
		WHERE id = $1 AND expires_at > NOW()`,
		id,
	)
	var s AdminSession
	err := row.Scan(&s.ID, &s.Username, &s.ExpiresAt, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (q *Queries) DeleteAdminSession(ctx context.Context, id string) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM admin_sessions WHERE id = $1`, id)
	return err
}

func (q *Queries) DeleteAdminSessionsByUsername(ctx context.Context, username string) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM admin_sessions WHERE username = $1`, username)
	return err
}

// ----- Login rate limiting -----

func (q *Queries) RecordLoginAttempt(ctx context.Context, username, ipAddress string, success bool) error {
	_, err := q.pool.Exec(ctx, `
		INSERT INTO login_attempts (username, ip_address, success) VALUES ($1, $2, $3)`,
		username, ipAddress, success,
	)
	return err
}

func (q *Queries) CountRecentFailedLoginAttemptsByIP(ctx context.Context, ipAddress string, since time.Time) (int64, error) {
	row := q.pool.QueryRow(ctx, `
		SELECT COUNT(*)::bigint FROM login_attempts
		WHERE ip_address = $1 AND success = false AND created_at >= $2`,
		ipAddress, since,
	)
	var n int64
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (q *Queries) CountRecentFailedLoginAttemptsByUsername(ctx context.Context, username string, since time.Time) (int64, error) {
	row := q.pool.QueryRow(ctx, `
		SELECT COUNT(*)::bigint FROM login_attempts
		WHERE username = $1 AND success = false AND created_at >= $2`,
		username, since,
	)
	var n int64
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
