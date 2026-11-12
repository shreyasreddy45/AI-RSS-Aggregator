-- +goose Up
ALTER TABLE posts
  ADD COLUMN IF NOT EXISTS content TEXT;

UPDATE posts
SET content = description
WHERE content IS NULL;

ALTER TABLE posts
  ADD COLUMN search_document tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
    setweight(to_tsvector('english', coalesce(description, '')), 'B') ||
    setweight(to_tsvector('english', coalesce(content, '')), 'C')
  ) STORED;

CREATE INDEX idx_posts_search_document
  ON posts USING GIN (search_document);

CREATE INDEX idx_posts_published_at
  ON posts (published_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_posts_published_at;
DROP INDEX IF EXISTS idx_posts_search_document;
ALTER TABLE posts DROP COLUMN IF EXISTS search_document;
ALTER TABLE posts DROP COLUMN IF EXISTS content;
