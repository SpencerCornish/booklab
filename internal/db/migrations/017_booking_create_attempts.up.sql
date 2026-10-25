CREATE TABLE booking_create_attempts (
    id         SERIAL PRIMARY KEY,
    ip_address TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_booking_create_attempts_ip ON booking_create_attempts (ip_address, created_at);