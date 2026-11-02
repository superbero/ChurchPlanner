-- +goose Up
ALTER TABLE songs
    ADD COLUMN youtube_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN original_key TEXT NOT NULL DEFAULT '';

UPDATE songs SET original_key = key WHERE original_key = '';

CREATE TABLE song_key_history (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    song_id    UUID NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
    used_on    DATE NOT NULL,
    key        TEXT NOT NULL,
    notes      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX song_key_history_song_idx ON song_key_history (song_id, used_on DESC);

-- +goose Down
DROP TABLE IF EXISTS song_key_history;
ALTER TABLE songs
    DROP COLUMN original_key,
    DROP COLUMN youtube_url;
