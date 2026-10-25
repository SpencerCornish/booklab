DROP INDEX IF EXISTS settings_secret_link_token_key;

ALTER TABLE bookings
  DROP COLUMN via_secret_link;

ALTER TABLE settings
  DROP COLUMN secret_link_any_hours,
  DROP COLUMN secret_link_token,
  DROP COLUMN secret_link_enabled;
