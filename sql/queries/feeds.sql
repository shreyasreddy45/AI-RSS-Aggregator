-- name: CreateFeed :one
INSERT INTO feeds (id,created_at,updated_at,name,url,user_id) 
values ($1,$2,$3,$4,$5,$6) 
ON CONFLICT (url) DO UPDATE SET url = feeds.url
RETURNING *;

-- name: GetFeeds :many
SELECT * FROM feeds ; 

-- name: GetNextFeedsToFetch :many
select * from feeds order by last_fetched_at ASC NULLS FIRST LIMIT $1;


-- name: MarkFeedasFetched :one
UPDATE feeds SET last_fetched_at = now(),
updated_at = now() WHERE id = $1 RETURNING *;