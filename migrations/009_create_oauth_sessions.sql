-- +goose Up

-- OAuth provider configurations stored in DB
CREATE TABLE IF NOT EXISTS oauth_providers (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name          VARCHAR(50) NOT NULL UNIQUE,   -- "google", "github", "okta", etc.
    type          VARCHAR(20) NOT NULL,           -- "oauth2" | "oidc"
    client_id     TEXT        NOT NULL,
    client_secret TEXT        NOT NULL,           -- AES-encrypted
    issuer_url    TEXT,                           -- OIDC only
    scopes        TEXT[]      NOT NULL DEFAULT '{}',
    is_enabled    BOOLEAN     NOT NULL DEFAULT true,
    settings      JSONB       NOT NULL DEFAULT '{}',  -- group_role_mapping, etc.
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Web sessions (stored as reference only; actual data lives in Redis)
-- This table is optional — used for auditing active sessions
CREATE TABLE IF NOT EXISTS sessions (
    id          VARCHAR(64) PRIMARY KEY,          -- random session token (stored in cookie)
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider    VARCHAR(50),                       -- which OAuth provider was used
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ NOT NULL,
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sessions_user    ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

-- OAuth state tokens (CSRF protection, short-lived)
CREATE TABLE IF NOT EXISTS oauth_states (
    state      VARCHAR(64) PRIMARY KEY,
    provider   VARCHAR(50) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '10 minutes'
);

-- +goose Down

DROP INDEX IF EXISTS idx_sessions_expires;
DROP INDEX IF EXISTS idx_sessions_user;
DROP TABLE IF EXISTS oauth_states;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS oauth_providers;
