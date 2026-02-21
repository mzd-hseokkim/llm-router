-- +goose Up

CREATE TABLE IF NOT EXISTS roles (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID        REFERENCES organizations(id) ON DELETE CASCADE,  -- NULL = system role
    name        VARCHAR(50) NOT NULL,
    description TEXT,
    is_system   BOOLEAN     NOT NULL DEFAULT false,   -- system roles cannot be deleted/modified
    permissions JSONB       NOT NULL DEFAULT '[]',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, name)
);

-- system roles (org_id = NULL, is_system = true)
INSERT INTO roles (id, org_id, name, description, is_system, permissions) VALUES
    (gen_random_uuid(), NULL, 'super_admin',  'Full system access',                      true, '["*"]'),
    (gen_random_uuid(), NULL, 'org_admin',    'Full org access — teams, users, budgets', true,
        '["keys:create","keys:read","keys:update","keys:delete","teams:manage","users:manage","usage:read","budget:set"]'),
    (gen_random_uuid(), NULL, 'team_admin',   'Team-scoped key and member management',   true,
        '["keys:create","keys:read","keys:update","keys:delete","teams:manage","usage:read"]'),
    (gen_random_uuid(), NULL, 'developer',    'Use models, view own usage',              true,
        '["keys:read","usage:read"]'),
    (gen_random_uuid(), NULL, 'viewer',       'Read-only access to own usage',           true,
        '["usage:read"]')
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS user_roles (
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id     UUID        NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    org_id      UUID        REFERENCES organizations(id) ON DELETE CASCADE,
    team_id     UUID        REFERENCES teams(id) ON DELETE CASCADE,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, role_id, org_id)
);

CREATE INDEX IF NOT EXISTS idx_user_roles_user  ON user_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_org   ON user_roles(org_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_team  ON user_roles(team_id);

-- +goose Down

DROP INDEX IF EXISTS idx_user_roles_team;
DROP INDEX IF EXISTS idx_user_roles_org;
DROP INDEX IF EXISTS idx_user_roles_user;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS roles;
