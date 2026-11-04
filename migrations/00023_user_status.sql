-- +goose Up
ALTER TABLE users ADD COLUMN status TEXT NOT NULL DEFAULT 'active';

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS status;
