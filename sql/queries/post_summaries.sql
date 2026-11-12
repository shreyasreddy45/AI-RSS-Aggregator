-- name: UpsertPostSummary :one
INSERT INTO post_summaries (
    id,
    post_id,
    summary,
    model,
    created_at,
    updated_at
)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (post_id)
DO UPDATE SET
    summary = EXCLUDED.summary,
    model = EXCLUDED.model,
    updated_at = NOW()
RETURNING *;

-- name: GetPostSummaryByPostID :one
SELECT *
FROM post_summaries
WHERE post_id = $1;

-- name: GetPostsWithoutSummaries :many
SELECT p.*
FROM posts p
LEFT JOIN post_summaries s
ON p.id = s.post_id
WHERE s.post_id IS NULL
ORDER BY p.created_at ASC;

-- name: GetPostSummariesForUser :many
SELECT ps.id, ps.post_id, ps.summary, ps.model, ps.created_at, ps.updated_at
FROM post_summaries ps
JOIN posts p ON p.id = ps.post_id
JOIN feed_follows ff ON ff.feed_id = p.feed_id
WHERE ff.user_id = $1
ORDER BY p.published_at DESC
LIMIT $2
OFFSET $3;
