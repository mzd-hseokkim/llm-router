-- +goose Up
CREATE TABLE provider_keys (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider            VARCHAR(50) NOT NULL,
    key_alias           VARCHAR(100) NOT NULL,
    encrypted_key       BYTEA NOT NULL,
    key_preview         VARCHAR(20),
    group_name          VARCHAR(100),
    tags                TEXT[],
    is_active           BOOLEAN DEFAULT true,
    weight              INTEGER DEFAULT 100,
    monthly_budget_usd  DECIMAL(12,4),
    current_month_spend DECIMAL(12,4) DEFAULT 0,
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW(),
    last_used_at        TIMESTAMPTZ,
    use_count           BIGINT DEFAULT 0
);

CREATE INDEX idx_provider_keys_provider ON provider_keys(provider) WHERE is_active = true;
CREATE INDEX idx_provider_keys_group ON provider_keys(provider, group_name) WHERE is_active = true;

-- +goose Down
DROP TABLE IF EXISTS provider_keys;
