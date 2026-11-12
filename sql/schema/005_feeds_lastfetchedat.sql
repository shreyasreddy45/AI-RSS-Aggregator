-- +goose Up

Alter table feeds Add column last_fetched_at TIMESTAMP ;




-- +goose Down

ALTER TABLE feeds DROP COLUMN last_fetched_at;
