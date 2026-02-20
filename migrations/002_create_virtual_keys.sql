-- +goose Up

CREATE TABLE IF NOT EXISTS virtual_keys (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    key_hash       VARCHAR(64)  NOT NULL UNIQUE,
    key_prefix     VARCHAR(10)  NOT NULL,
    name           VARCHAR(255),
    user_id        UUID         REFERENCES users(id)         ON DELETE SET NULL,
    team_id        UUID         REFERENCES teams(id)         ON DELETE SET NULL,
    org_id         UUID         REFERENCES organizations(id) ON DELETE SET NULL,
    expires_at     TIMESTAMPTZ,
    budget_usd     DECIMAL(12,4),
    rpm_limit      INTEGER,
    tpm_limit      INTEGER,
    allowed_models TEXT[],
    blocked_models TEXT[],
    metadata       JSONB        NOT NULL DEFAULT '{}',
    is_active      BOOLEAN      NOT NULL DEFAULT true,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_used_at   TIMESTAMPTZ
);

CREATE INDEX idx_virtual_keys_prefix ON virtual_keys(key_prefix);

-- +goose Down

DROP TABLE IF EXISTS virtual_keys;
