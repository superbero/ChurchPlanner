-- +goose Up
ALTER TABLE users ADD COLUMN title TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS title;
