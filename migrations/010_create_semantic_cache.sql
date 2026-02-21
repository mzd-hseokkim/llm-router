-- +goose Up
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE semantic_cache (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    model             VARCHAR(200) NOT NULL,
    embedding         vector(1536),           -- OpenAI text-embedding-3-small dimensions
    prompt_text       TEXT        NOT NULL,
    response_json     JSONB       NOT NULL,
    prompt_tokens     INTEGER,
    completion_tokens INTEGER,
    cost_usd          DECIMAL(12, 8),
    hit_count         INTEGER     NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at        TIMESTAMPTZ NOT NULL
);

-- IVFFlat index for approximate nearest-neighbor cosine similarity search.
-- lists=100 is suitable for up to ~100k entries; re-tune (VACUUM ANALYZE) after bulk inserts.
CREATE INDEX idx_semantic_cache_embedding ON semantic_cache
    USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);

CREATE INDEX idx_semantic_cache_model_expires ON semantic_cache (model, expires_at);

-- +goose Down
DROP INDEX IF EXISTS idx_semantic_cache_model_expires;
DROP INDEX IF EXISTS idx_semantic_cache_embedding;
DROP TABLE IF EXISTS semantic_cache;
