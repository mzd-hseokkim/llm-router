-- +goose Up

CREATE TABLE request_logs (
    id                UUID        NOT NULL DEFAULT gen_random_uuid(),
    request_id        VARCHAR(36) NOT NULL,
    timestamp         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    model             VARCHAR(200) NOT NULL,
    provider          VARCHAR(50) NOT NULL,
    virtual_key_id    UUID        REFERENCES virtual_keys(id),
    user_id           UUID,
    team_id           UUID,
    org_id            UUID,
    prompt_tokens     INTEGER,
    completion_tokens INTEGER,
    total_tokens      INTEGER,
    cost_usd          DECIMAL(12,8),
    latency_ms        INTEGER,
    ttft_ms           INTEGER,
    status_code       SMALLINT,
    finish_reason     VARCHAR(20),
    cache_hit         BOOLEAN     NOT NULL DEFAULT false,
    is_streaming      BOOLEAN     NOT NULL DEFAULT false,
    error_code        VARCHAR(50),
    error_message     TEXT,
    metadata          JSONB       NOT NULL DEFAULT '{}'
) PARTITION BY RANGE (timestamp);

-- Monthly partitions for 2026
CREATE TABLE request_logs_2026_01 PARTITION OF request_logs
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

CREATE TABLE request_logs_2026_02 PARTITION OF request_logs
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');

CREATE TABLE request_logs_2026_03 PARTITION OF request_logs
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');

CREATE TABLE request_logs_2026_04 PARTITION OF request_logs
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');

CREATE TABLE request_logs_2026_05 PARTITION OF request_logs
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');

CREATE TABLE request_logs_2026_06 PARTITION OF request_logs
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

CREATE TABLE request_logs_2026_07 PARTITION OF request_logs
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

CREATE TABLE request_logs_2026_08 PARTITION OF request_logs
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');

CREATE TABLE request_logs_2026_09 PARTITION OF request_logs
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');

CREATE TABLE request_logs_2026_10 PARTITION OF request_logs
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');

CREATE TABLE request_logs_2026_11 PARTITION OF request_logs
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');

CREATE TABLE request_logs_2026_12 PARTITION OF request_logs
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

CREATE INDEX idx_logs_timestamp     ON request_logs (timestamp DESC);
CREATE INDEX idx_logs_virtual_key   ON request_logs (virtual_key_id, timestamp DESC);
CREATE INDEX idx_logs_model         ON request_logs (model, timestamp DESC);

-- +goose Down

DROP TABLE IF EXISTS request_logs;
