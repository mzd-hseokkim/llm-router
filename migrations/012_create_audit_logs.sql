-- +goose Up

CREATE TABLE audit_logs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type    VARCHAR(100) NOT NULL,   -- 'virtual_key.created', 'user.deleted'
    action        VARCHAR(50)  NOT NULL,   -- 'create', 'update', 'delete', 'login'
    actor_type    VARCHAR(20)  NOT NULL,   -- 'user', 'api_key', 'system'
    actor_id      UUID,
    actor_email   VARCHAR(255),
    ip_address    INET,
    user_agent    TEXT,
    resource_type VARCHAR(50),
    resource_id   UUID,
    resource_name VARCHAR(255),
    changes       JSONB,
    metadata      JSONB DEFAULT '{}',
    request_id    VARCHAR(36),
    org_id        UUID,
    team_id       UUID,
    timestamp     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_timestamp    ON audit_logs (timestamp DESC);
CREATE INDEX idx_audit_logs_event_type   ON audit_logs (event_type);
CREATE INDEX idx_audit_logs_actor_id     ON audit_logs (actor_id) WHERE actor_id IS NOT NULL;
CREATE INDEX idx_audit_logs_resource     ON audit_logs (resource_type, resource_id) WHERE resource_id IS NOT NULL;
CREATE INDEX idx_audit_logs_org_id       ON audit_logs (org_id) WHERE org_id IS NOT NULL;

-- Immutability: deny UPDATE and DELETE at RLS level.
ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
CREATE POLICY audit_log_select ON audit_logs FOR SELECT USING (true);
-- No UPDATE or DELETE policies → those operations are blocked by default.

-- +goose Down

DROP TABLE IF EXISTS audit_logs;
