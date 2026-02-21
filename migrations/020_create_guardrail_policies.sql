-- +goose Up
CREATE TABLE guardrail_policies (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    guardrail_type VARCHAR(50) NOT NULL UNIQUE,  -- pii, prompt_injection, content_filter, custom_keywords, llm_judge
    is_enabled     BOOLEAN NOT NULL DEFAULT false,
    action         VARCHAR(20) NOT NULL DEFAULT 'log_only',  -- block, mask, log_only
    engine         VARCHAR(20),                               -- regex, llm (nullable)
    config_json    JSONB NOT NULL DEFAULT '{}',
    sort_order     INTEGER DEFAULT 0,
    created_at     TIMESTAMPTZ DEFAULT NOW(),
    updated_at     TIMESTAMPTZ DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS guardrail_policies;
