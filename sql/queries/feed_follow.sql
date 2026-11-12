-- name: CreateFeedFollow :one
INSERT INTO feed_follows (id,created_at,updated_at,user_id,feed_id) 
values ($1,$2,$3,$4,$5) 
ON CONFLICT (user_id, feed_id) DO UPDATE SET id = feed_follows.id
RETURNING *;

-- name: GetFeedFollows :many
SELECT * FROM feed_follows WHERE user_id = $1;

-- name: DeleteFeedFollow :exec
DELETE FROM feed_follows WHERE id = $1 AND user_id = $2;