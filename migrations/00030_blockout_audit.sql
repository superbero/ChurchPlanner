-- +goose Up
-- Who created / last changed a blockout, so "added by an admin" can be shown.
ALTER TABLE user_blockouts
    ADD COLUMN created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN updated_by UUID REFERENCES users(id) ON DELETE SET NULL;

-- Existing rows were all added by the person themselves.
UPDATE user_blockouts SET created_by = user_id WHERE created_by IS NULL;

-- +goose Down
ALTER TABLE user_blockouts
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by;
