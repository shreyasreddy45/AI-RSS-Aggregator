-- +goose Up

CREATE TABLE feeds(
    id UUID PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    url TEXT UNIQUE NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) on delete cascade
);


-- +goose Down

DROP TABLE feeds;