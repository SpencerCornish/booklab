-- Enable btree_gist for exclusion constraints on ranges
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- Booking status enum
CREATE TYPE booking_status AS ENUM ('confirmed', 'cancelled', 'completed', 'charged');

-- Single-row settings table
CREATE TABLE settings (
    id           INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    resource_name          TEXT    NOT NULL DEFAULT 'The Space',
    hourly_rate_cents      INT     NOT NULL DEFAULT 1500,
    currency               TEXT    NOT NULL DEFAULT 'usd',
    timezone               TEXT    NOT NULL DEFAULT 'America/Denver',
    bookable_start         TIME    NOT NULL DEFAULT '09:00',
    bookable_end           TIME    NOT NULL DEFAULT '21:00',
    min_hours              INT     NOT NULL DEFAULT 1,
    max_hours              INT     NOT NULL DEFAULT 8,
    reminder_hours_before  INT     NOT NULL DEFAULT 24
);

INSERT INTO settings DEFAULT VALUES;

-- Closures: date ranges when the resource is unavailable
CREATE TABLE closures (
    id          SERIAL PRIMARY KEY,
    start_date  DATE        NOT NULL,
    end_date    DATE        NOT NULL,
    reason      TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT closures_date_order CHECK (end_date >= start_date)
);

-- Bookings
CREATE TABLE bookings (
    id                        SERIAL PRIMARY KEY,
    name                      TEXT           NOT NULL,
    email                     TEXT           NOT NULL,
    start_time                TIMESTAMPTZ    NOT NULL,
    end_time                  TIMESTAMPTZ    NOT NULL,
    status                    booking_status NOT NULL DEFAULT 'confirmed',
    cancel_token              UUID           NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    stripe_setup_intent_id    TEXT,
    stripe_payment_method_id  TEXT,
    stripe_payment_intent_id  TEXT,
    amount_cents              INT,
    reminder_sent             BOOLEAN        NOT NULL DEFAULT FALSE,
    created_at                TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT bookings_time_order CHECK (end_time > start_time)
);

-- DB-level conflict prevention: no two active bookings can overlap
CREATE EXTENSION IF NOT EXISTS btree_gist;
ALTER TABLE bookings
    ADD CONSTRAINT bookings_no_overlap
    EXCLUDE USING gist (
        tstzrange(start_time, end_time) WITH &&
    )
    WHERE (status NOT IN ('cancelled'));

-- Admin users
CREATE TABLE admin_users (
    id            SERIAL PRIMARY KEY,
    username      TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Auto-update updated_at on bookings
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER bookings_updated_at
    BEFORE UPDATE ON bookings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
