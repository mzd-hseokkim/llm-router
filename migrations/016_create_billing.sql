-- +goose Up

-- Per-team markup configuration (NULL team_id = global default).
CREATE TABLE markup_configs (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id     UUID         UNIQUE,        -- NULL = global default
    percentage  DECIMAL(8,2) NOT NULL DEFAULT 0,   -- e.g. 20 = 20% markup
    fixed_usd   DECIMAL(12,8) NOT NULL DEFAULT 0,  -- per-request fixed charge
    cap_usd     DECIMAL(12,8),                     -- NULL = no cap
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Global default markup row (team_id IS NULL, zero markup).
INSERT INTO markup_configs (percentage, fixed_usd) VALUES (0, 0);

-- +goose Down

DROP TABLE IF EXISTS markup_configs;
