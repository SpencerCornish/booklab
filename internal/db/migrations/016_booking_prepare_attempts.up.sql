CREATE TABLE booking_prepare_attempts (
    id         SERIAL PRIMARY KEY,
    ip_address TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_booking_prepare_attempts_ip ON booking_prepare_attempts (ip_address, created_at);
