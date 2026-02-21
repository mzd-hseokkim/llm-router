-- +goose Up

CREATE TABLE routing_rules (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(100) NOT NULL UNIQUE,
    priority    INTEGER NOT NULL DEFAULT 0,
    enabled     BOOLEAN NOT NULL DEFAULT true,

    -- Match conditions (NULL means "match anything")
    match_model              VARCHAR(255),   -- exact model name
    match_model_prefix       VARCHAR(255),   -- prefix match (e.g. "openai/")
    match_model_regex        VARCHAR(255),   -- regex match

    match_key_id             UUID,
    match_user_id            UUID,
    match_team_id            UUID,
    match_org_id             UUID,

    match_metadata           JSONB,          -- {"user_tier": "premium"}
    match_min_context_tokens INTEGER,        -- min estimated tokens in messages
    match_max_context_tokens INTEGER,
    match_has_tools          BOOLEAN,        -- request uses tool_calls

    -- Strategy: direct | weighted | least_cost | failover | quality_first
    strategy    VARCHAR(50) NOT NULL DEFAULT 'direct',

    -- Targets: [{provider, model, weight}]
    targets     JSONB NOT NULL DEFAULT '[]',

    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for priority-ordered rule evaluation
CREATE INDEX routing_rules_priority_idx ON routing_rules (priority DESC)
    WHERE enabled = true;

-- +goose Down

DROP INDEX IF EXISTS routing_rules_priority_idx;
DROP TABLE IF EXISTS routing_rules;
