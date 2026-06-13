-- name: CreateBooking :one
INSERT INTO bookings (name, email, start_time, end_time, stripe_setup_intent_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetBookingByID :one
SELECT * FROM bookings WHERE id = $1;

-- name: GetBookingByToken :one
SELECT * FROM bookings WHERE cancel_token = $1;

-- name: ListBookings :many
SELECT * FROM bookings
WHERE
    ($1::date IS NULL OR start_time::date = $1)
    AND ($2::booking_status IS NULL OR status = $2)
ORDER BY start_time DESC;

-- name: ListBookingsInRange :many
SELECT * FROM bookings
WHERE start_time >= $1 AND start_time < $2
  AND status != 'cancelled'
ORDER BY start_time;

-- name: ListBookingsDueReminder :many
SELECT * FROM bookings
WHERE reminder_sent = FALSE
  AND status = 'confirmed'
  AND start_time BETWEEN NOW() AND NOW() + ($1 || ' hours')::interval
  AND start_time - created_at >= ($1 || ' hours')::interval;

-- name: UpdateBookingStatus :one
UPDATE bookings SET status = $2 WHERE id = $1 RETURNING *;

-- name: UpdateBookingEndTime :one
UPDATE bookings SET end_time = $2 WHERE id = $1 RETURNING *;

-- name: UpdateBookingPaymentMethod :one
UPDATE bookings SET stripe_payment_method_id = $2 WHERE id = $1 RETURNING *;

-- name: UpdateBookingCharged :one
UPDATE bookings SET
    status = 'charged',
    stripe_payment_intent_id = $2,
    amount_cents = $3
WHERE id = $1
RETURNING *;

-- name: MarkReminderSent :exec
UPDATE bookings SET reminder_sent = TRUE WHERE id = $1;

-- name: CancelBooking :one
UPDATE bookings SET status = 'cancelled' WHERE cancel_token = $1 AND status = 'confirmed' RETURNING *;
