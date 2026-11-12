-- +goose Up

CREATE TABLE post_summaries (
    id UUID PRIMARY KEY,
    post_id UUID NOT NULL UNIQUE REFERENCES posts(id) ON DELETE CASCADE,
    summary TEXT NOT NULL,
    model VARCHAR(128) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_post_summaries_post_id ON post_summaries(post_id);

-- +goose Down
DROP TABLE IF EXISTS post_summaries;
