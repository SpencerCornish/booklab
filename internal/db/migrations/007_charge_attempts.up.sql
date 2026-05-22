ALTER TABLE bookings
    ADD COLUMN charge_attempts   INT  NOT NULL DEFAULT 0,
    ADD COLUMN last_charge_error TEXT;
