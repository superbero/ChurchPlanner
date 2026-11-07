-- Additive migration (safe per the README update procedure).
-- ASSUMPTION: users.id is uuid. If it is bigint/text, change user_id's type.
CREATE TABLE IF NOT EXISTS calendar_feed_tokens (
    user_id    uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    token      text NOT NULL UNIQUE,   -- 43 chars, base64url of 32 random bytes
    created_at timestamptz NOT NULL DEFAULT now()
);
-- UNIQUE already creates the index the feed lookup (WHERE token = $1) needs.
