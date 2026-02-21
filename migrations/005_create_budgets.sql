-- +goose Up

CREATE TABLE IF NOT EXISTS budgets (
    id             UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type    VARCHAR(20)   NOT NULL,  -- 'key', 'user', 'team', 'org'
    entity_id      UUID          NOT NULL,
    period         VARCHAR(10)   NOT NULL,  -- 'hourly', 'daily', 'weekly', 'monthly', 'lifetime'
    soft_limit_usd DECIMAL(12,4),           -- NULL = 미설정
    hard_limit_usd DECIMAL(12,4),           -- NULL = 무제한
    current_spend  DECIMAL(12,8) NOT NULL DEFAULT 0,
    period_start   TIMESTAMPTZ   NOT NULL,
    period_end     TIMESTAMPTZ   NOT NULL,
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE (entity_type, entity_id, period)
);

CREATE INDEX idx_budgets_entity ON budgets(entity_type, entity_id);
CREATE INDEX idx_budgets_period_end ON budgets(period_end) WHERE period != 'lifetime';

-- +goose Down

DROP TABLE IF EXISTS budgets;
