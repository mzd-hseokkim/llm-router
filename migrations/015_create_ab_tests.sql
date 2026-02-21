-- +goose Up

CREATE TABLE ab_tests (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name             VARCHAR(200) NOT NULL,
    status           VARCHAR(20)  NOT NULL DEFAULT 'draft',
    traffic_split    JSONB        NOT NULL DEFAULT '[]',
    target           JSONB        NOT NULL DEFAULT '{}',
    success_metrics  TEXT[]       NOT NULL DEFAULT '{}',
    min_samples      INTEGER      NOT NULL DEFAULT 1000,
    confidence_level DECIMAL(4,2) NOT NULL DEFAULT 0.95,
    start_at         TIMESTAMPTZ,
    end_at           TIMESTAMPTZ,
    winner           VARCHAR(50),
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT ab_tests_status_check
        CHECK (status IN ('draft','running','paused','completed','stopped'))
);

CREATE TABLE ab_test_results (
    test_id           UUID        NOT NULL REFERENCES ab_tests(id) ON DELETE CASCADE,
    variant           VARCHAR(50) NOT NULL,
    request_id        UUID        NOT NULL,
    timestamp         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    model             VARCHAR(200),
    latency_ms        INTEGER,
    prompt_tokens     INTEGER,
    completion_tokens INTEGER,
    cost_usd          DECIMAL(12,8),
    error             BOOLEAN     NOT NULL DEFAULT false,
    finish_reason     VARCHAR(20),
    PRIMARY KEY (test_id, request_id)
);

CREATE INDEX idx_ab_test_results_variant ON ab_test_results(test_id, variant);
CREATE INDEX idx_ab_test_results_ts      ON ab_test_results(test_id, timestamp DESC);

-- +goose Down

DROP TABLE IF EXISTS ab_test_results;
DROP TABLE IF EXISTS ab_tests;
