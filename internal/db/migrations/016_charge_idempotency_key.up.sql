-- The Stripe idempotency key for the booking's current charge attempt. It is kept after a
-- charge fails with an unknown outcome, so the retry replays the original request instead
-- of charging again, and cleared once Stripe has definitely declined.
ALTER TABLE bookings
    ADD COLUMN charge_idempotency_key TEXT;
