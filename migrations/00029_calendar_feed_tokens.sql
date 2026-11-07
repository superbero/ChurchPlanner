-- +goose Up
-- ASSUMPTION: users.id is uuid. If it is bigint/text, change user_id's type.
CREATE TABLE IF NOT EXISTS calendar_feed_tokens (
    user_id    uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    token      text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS calendar_feed_tokens;
