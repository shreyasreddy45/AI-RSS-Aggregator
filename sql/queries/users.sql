-- name: CreateUser :one
INSERT INTO users (
    id,
    created_at,
    updated_at,
    name,
    email,
    password_hash,
    role,
    api_key
)
VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    DEFAULT,
    encode(sha256(random()::text::bytea), 'hex')
)
RETURNING *;


-- name: GetUserByApiKey :one
SELECT *
FROM users
WHERE api_key = $1;


-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = $1;


-- name: UpdateUserRole :one
UPDATE users
SET
    role = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = $1;