ALTER TABLE settings ADD COLUMN notification_emails TEXT NOT NULL DEFAULT '';
ALTER TABLE bookings ADD COLUMN stripe_receipt_url TEXT;
ALTER TABLE bookings ADD COLUMN completed_at TIMESTAMPTZ;
