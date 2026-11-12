-- name: CreatePost :one
INSERT INTO posts (
    id,
    title,
    description,
    created_at,
    updated_at,
    published_at,
    url,
    feed_id
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING *;

-- name: GetPostsForUser :many
SELECT posts.*
FROM posts
JOIN feed_follows
ON posts.feed_id = feed_follows.feed_id
WHERE feed_follows.user_id = sqlc.arg(user_id)
AND LOWER(posts.title) LIKE '%' || LOWER(sqlc.arg(search)) || '%'
ORDER BY posts.published_at DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: GetPostByIDForUser :one
SELECT p.id, p.title, p.description, p.content, p.created_at, p.updated_at, p.published_at, p.url, p.feed_id
FROM posts p
JOIN feed_follows ff ON ff.feed_id = p.feed_id
WHERE ff.user_id = $1 AND p.id = $2
LIMIT 1;
