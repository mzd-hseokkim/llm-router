-- +goose Up
-- +goose StatementBegin

CREATE TABLE prompts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        VARCHAR(100) NOT NULL,
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    team_id     UUID,
    visibility  VARCHAR(20) NOT NULL DEFAULT 'team',  -- private | team | public
    created_by  UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX prompts_slug_team_idx ON prompts (slug, COALESCE(team_id, '00000000-0000-0000-0000-000000000000'::UUID));

CREATE TABLE prompt_versions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prompt_id   UUID NOT NULL REFERENCES prompts(id) ON DELETE CASCADE,
    version     VARCHAR(20) NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'draft',  -- draft | active | deprecated | archived
    template    TEXT NOT NULL,
    variables   JSONB NOT NULL DEFAULT '[]',
    parameters  JSONB NOT NULL DEFAULT '{}',
    model       VARCHAR(200),
    changelog   TEXT,
    created_by  UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (prompt_id, version)
);

CREATE INDEX prompt_versions_prompt_id_idx ON prompt_versions (prompt_id);
CREATE INDEX prompt_versions_status_idx ON prompt_versions (status);

-- Tracks per-day usage metrics for each (prompt, version) pair.
CREATE TABLE prompt_metrics (
    prompt_id       UUID NOT NULL REFERENCES prompts(id) ON DELETE CASCADE,
    prompt_version  VARCHAR(20) NOT NULL,
    date            DATE NOT NULL,
    request_count   INTEGER NOT NULL DEFAULT 0,
    avg_latency_ms  FLOAT,
    avg_tokens      FLOAT,
    total_cost_usd  DECIMAL(14,8) NOT NULL DEFAULT 0,
    error_count     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (prompt_id, prompt_version, date)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS prompt_metrics;
DROP TABLE IF EXISTS prompt_versions;
DROP TABLE IF EXISTS prompts;
-- +goose StatementEnd
