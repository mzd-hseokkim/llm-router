-- +goose Up

-- Sentinel UUID used when virtual_key_id / user_id / team_id is unknown.
-- '00000000-0000-0000-0000-000000000000' represents NULL in composite PK.

CREATE TABLE IF NOT EXISTS daily_usage (
    date              DATE         NOT NULL,
    model             VARCHAR(200) NOT NULL,
    provider          VARCHAR(50)  NOT NULL,
    virtual_key_id    UUID         NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    user_id           UUID,
    team_id           UUID,
    org_id            UUID,
    request_count     INTEGER      NOT NULL DEFAULT 0,
    prompt_tokens     BIGINT       NOT NULL DEFAULT 0,
    completion_tokens BIGINT       NOT NULL DEFAULT 0,
    total_tokens      BIGINT       NOT NULL DEFAULT 0,
    cost_usd          DECIMAL(14,8) NOT NULL DEFAULT 0,
    error_count       INTEGER      NOT NULL DEFAULT 0,
    PRIMARY KEY (date, model, provider, virtual_key_id)
);

CREATE INDEX idx_daily_usage_date ON daily_usage(date DESC);
CREATE INDEX idx_daily_usage_key ON daily_usage(virtual_key_id, date DESC)
    WHERE virtual_key_id != '00000000-0000-0000-0000-000000000000';
CREATE INDEX idx_daily_usage_team ON daily_usage(team_id, date DESC) WHERE team_id IS NOT NULL;

-- +goose Down

DROP TABLE IF EXISTS daily_usage;
