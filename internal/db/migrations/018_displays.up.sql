-- E-ink door-sign displays running TRMNL firmware pointed at this server.
CREATE TABLE displays (
    id               SERIAL PRIMARY KEY,
    mac_address      TEXT NOT NULL UNIQUE,
    -- SHA-256 (hex) of the device's API key; the key itself is never stored.
    api_key_hash     TEXT NOT NULL UNIQUE,
    friendly_id      TEXT NOT NULL UNIQUE,
    -- Secret path segment for the image URL, which the device fetches without headers.
    image_token      UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    name             TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'revoked')),
    model            TEXT NOT NULL DEFAULT '',
    firmware_version TEXT NOT NULL DEFAULT '',
    battery_voltage  REAL,
    rssi             INTEGER,
    width            INTEGER,
    height           INTEGER,
    last_seen_at     TIMESTAMPTZ,
    last_ip          TEXT NOT NULL DEFAULT '',
    approved_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE settings
    ADD COLUMN display_refresh_minutes INTEGER NOT NULL DEFAULT 5
        CHECK (display_refresh_minutes BETWEEN 1 AND 60);
