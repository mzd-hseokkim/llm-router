-- +goose Up

CREATE TABLE alert_history (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type  VARCHAR(100) NOT NULL,
    severity    VARCHAR(20)  NOT NULL,
    channel     VARCHAR(100) NOT NULL,
    status      VARCHAR(20)  NOT NULL,  -- 'sent', 'failed', 'deduplicated'
    payload     JSONB        NOT NULL DEFAULT '{}',
    error       TEXT,
    sent_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_alert_history_sent_at    ON alert_history (sent_at DESC);
CREATE INDEX idx_alert_history_event_type ON alert_history (event_type);
CREATE INDEX idx_alert_history_status     ON alert_history (status);

-- +goose Down

DROP TABLE IF EXISTS alert_history;
