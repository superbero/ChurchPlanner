-- +goose Up
ALTER TABLE users
    ADD COLUMN phone TEXT NOT NULL DEFAULT '',
    ADD COLUMN vocal_low TEXT NOT NULL DEFAULT '',
    ADD COLUMN vocal_high TEXT NOT NULL DEFAULT '';

CREATE TABLE user_positions (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    position_id UUID NOT NULL REFERENCES positions(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, position_id)
);

-- +goose Down
DROP TABLE IF EXISTS user_positions;
ALTER TABLE users
    DROP COLUMN IF EXISTS phone,
    DROP COLUMN IF EXISTS vocal_low,
    DROP COLUMN IF EXISTS vocal_high;
