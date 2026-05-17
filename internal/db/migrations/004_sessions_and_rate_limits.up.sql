CREATE TABLE admin_sessions (
    id         TEXT PRIMARY KEY,
    username   TEXT NOT NULL REFERENCES admin_users (username) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_admin_sessions_username ON admin_sessions (username);

CREATE TABLE login_attempts (
    id         SERIAL PRIMARY KEY,
    username   TEXT NOT NULL,
    ip_address TEXT NOT NULL,
    success    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_login_attempts_ip ON login_attempts (ip_address, created_at);
CREATE INDEX idx_login_attempts_user ON login_attempts (username, created_at);
