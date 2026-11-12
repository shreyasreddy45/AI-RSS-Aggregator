-- +goose Up

-- Step 1: Add new columns (nullable initially)
ALTER TABLE users
ADD COLUMN email VARCHAR(255),
ADD COLUMN password_hash TEXT,
ADD COLUMN role VARCHAR(20) DEFAULT 'user';

-- Step 2: Populate existing rows
UPDATE users
SET
    email = id::text || '@placeholder.local',
    password_hash = '',
    role = 'user'
WHERE email IS NULL;

-- Step 3: Add constraints
ALTER TABLE users
ALTER COLUMN email SET NOT NULL,
ALTER COLUMN password_hash SET NOT NULL;

ALTER TABLE users
ADD CONSTRAINT users_email_unique UNIQUE (email);

ALTER TABLE users
ADD CONSTRAINT users_role_check
CHECK (role IN ('user', 'admin'));

-- +goose Down

ALTER TABLE users
DROP CONSTRAINT IF EXISTS users_role_check;

ALTER TABLE users
DROP CONSTRAINT IF EXISTS users_email_unique;

ALTER TABLE users
DROP COLUMN role,
DROP COLUMN password_hash,
DROP COLUMN email;