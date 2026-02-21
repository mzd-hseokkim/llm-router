-- +goose Up

CREATE TABLE providers (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         VARCHAR(50)  NOT NULL UNIQUE,
    adapter_type VARCHAR(50)  NOT NULL,
    display_name VARCHAR(100) NOT NULL DEFAULT '',
    base_url     TEXT,
    is_enabled   BOOLEAN NOT NULL DEFAULT true,
    config_json  JSONB,
    sort_order   INTEGER DEFAULT 0,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE models (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id               UUID NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    model_id                  VARCHAR(200) NOT NULL,
    model_name                VARCHAR(200) NOT NULL,
    display_name              VARCHAR(200) DEFAULT '',
    is_enabled                BOOLEAN NOT NULL DEFAULT true,
    input_per_million_tokens  DECIMAL(12,6) DEFAULT 0,
    output_per_million_tokens DECIMAL(12,6) DEFAULT 0,
    context_window            INTEGER,
    max_output_tokens         INTEGER,
    supports_streaming        BOOLEAN DEFAULT true,
    supports_tools            BOOLEAN DEFAULT false,
    supports_vision           BOOLEAN DEFAULT false,
    tags                      TEXT[],
    sort_order                INTEGER DEFAULT 0,
    created_at                TIMESTAMPTZ DEFAULT NOW(),
    updated_at                TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(provider_id, model_id)
);

CREATE INDEX idx_models_provider_id ON models(provider_id);
CREATE INDEX idx_providers_name ON providers(name);

-- +goose Down

DROP TABLE IF EXISTS models;
DROP TABLE IF EXISTS providers;
