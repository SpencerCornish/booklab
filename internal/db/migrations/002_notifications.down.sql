ALTER TABLE settings DROP COLUMN IF EXISTS notification_emails;
ALTER TABLE bookings DROP COLUMN IF EXISTS stripe_receipt_url;
ALTER TABLE bookings DROP COLUMN IF EXISTS completed_at;
