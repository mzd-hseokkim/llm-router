-- +goose Up

-- Add slug, settings, is_active to organizations
ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS slug       VARCHAR(100) UNIQUE,
    ADD COLUMN IF NOT EXISTS settings   JSONB        NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS is_active  BOOLEAN      NOT NULL DEFAULT true;

-- Backfill slugs from name for existing rows
UPDATE organizations SET slug = LOWER(REGEXP_REPLACE(name, '[^a-zA-Z0-9]+', '-', 'g'))
WHERE slug IS NULL;

ALTER TABLE organizations ALTER COLUMN slug SET NOT NULL;

-- Add slug, settings to teams
ALTER TABLE teams
    ADD COLUMN IF NOT EXISTS slug       VARCHAR(100),
    ADD COLUMN IF NOT EXISTS settings   JSONB        NOT NULL DEFAULT '{}';

-- Backfill slugs for existing teams
UPDATE teams SET slug = LOWER(REGEXP_REPLACE(name, '[^a-zA-Z0-9]+', '-', 'g'))
WHERE slug IS NULL;

ALTER TABLE teams ALTER COLUMN slug SET NOT NULL;
ALTER TABLE teams ADD CONSTRAINT teams_org_slug_unique UNIQUE (org_id, slug);

-- Add password_hash to users (nullable — used for local auth; SSO users won't have one)
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS name          VARCHAR(255),
    ADD COLUMN IF NOT EXISTS password_hash VARCHAR(255),
    ADD COLUMN IF NOT EXISTS is_active     BOOLEAN NOT NULL DEFAULT true;

-- Many-to-many: users ↔ teams (replaces the single team_id column)
-- team_id on users is kept for backwards compatibility but team_members is the primary relation
CREATE TABLE IF NOT EXISTS team_members (
    team_id   UUID        NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id   UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role      VARCHAR(50) NOT NULL DEFAULT 'developer',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (team_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_team_members_user ON team_members(user_id);

-- Org-level daily usage view (org_id must exist on request_logs)
-- request_logs already has org_id; add index if missing
CREATE INDEX IF NOT EXISTS idx_request_logs_org_id ON request_logs(org_id) WHERE org_id IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_request_logs_org_id;
DROP INDEX IF EXISTS idx_team_members_user;
DROP TABLE IF EXISTS team_members;

ALTER TABLE users DROP COLUMN IF EXISTS is_active;
ALTER TABLE users DROP COLUMN IF EXISTS password_hash;
ALTER TABLE users DROP COLUMN IF EXISTS name;

ALTER TABLE teams DROP CONSTRAINT IF EXISTS teams_org_slug_unique;
ALTER TABLE teams DROP COLUMN IF EXISTS settings;
ALTER TABLE teams DROP COLUMN IF EXISTS slug;

ALTER TABLE organizations DROP COLUMN IF EXISTS is_active;
ALTER TABLE organizations DROP COLUMN IF EXISTS settings;
ALTER TABLE organizations DROP COLUMN IF EXISTS slug;
