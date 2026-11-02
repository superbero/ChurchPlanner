-- +goose Up
ALTER TABLE plan_people ADD COLUMN notified_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE plan_people DROP COLUMN IF EXISTS notified_at;
