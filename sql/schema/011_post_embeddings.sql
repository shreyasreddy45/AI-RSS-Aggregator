-- +goose Up

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE post_embeddings (
    id UUID PRIMARY KEY,
    post_id UUID NOT NULL UNIQUE REFERENCES posts(id) ON DELETE CASCADE,
    embedding vector(1536) NOT NULL,
    provider VARCHAR(32) NOT NULL DEFAULT 'gemini',
    model VARCHAR(128) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_post_embeddings_vector_hnsw
    ON post_embeddings
    USING hnsw (embedding vector_cosine_ops);

CREATE INDEX idx_post_embeddings_post_id
    ON post_embeddings(post_id);

-- +goose Down
DROP TABLE IF EXISTS post_embeddings;
DROP EXTENSION IF EXISTS vector;
