-- Secret booking link (free studio-artist bookings).
-- A settings toggle exposes a secret link that books without Stripe, without
-- the minimum-lead-time rule, and (optionally) outside regular bookable hours.
ALTER TABLE settings
  ADD COLUMN secret_link_enabled   BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN secret_link_token     TEXT,          -- NULL until first enable/rotate
  ADD COLUMN secret_link_any_hours BOOLEAN NOT NULL DEFAULT FALSE;

-- Tokens are looked up by equality; the partial unique index keeps NULLs out
-- and makes a leaked token's replacement collide loudly if reused.
CREATE UNIQUE INDEX settings_secret_link_token_key
  ON settings (secret_link_token) WHERE secret_link_token IS NOT NULL;

-- Free bookings carry no Stripe identifiers; via_secret_link is the marker the
-- lifecycle (auto-complete, charge guards) and admin UI key off.
ALTER TABLE bookings
  ADD COLUMN via_secret_link BOOLEAN NOT NULL DEFAULT FALSE;
