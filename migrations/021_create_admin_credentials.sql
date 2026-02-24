-- +goose Up
CREATE TABLE IF NOT EXISTS admin_credentials (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username         VARCHAR(50) NOT NULL UNIQUE DEFAULT 'admin',
    password_hash    TEXT NOT NULL,
    password_changed BOOLEAN NOT NULL DEFAULT false,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS admin_credentials;
