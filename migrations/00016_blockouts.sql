-- +goose Up
CREATE TABLE user_blockouts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    starts_on   DATE NOT NULL,
    ends_on     DATE NOT NULL,
    reason      TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX user_blockouts_user_idx ON user_blockouts (user_id, starts_on, ends_on);

-- +goose Down
DROP TABLE IF EXISTS user_blockouts;
