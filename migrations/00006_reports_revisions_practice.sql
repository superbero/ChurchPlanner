-- +goose Up
CREATE TABLE song_arrangement_revisions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    arrangement_id  UUID NOT NULL REFERENCES song_arrangements(id) ON DELETE CASCADE,
    name            TEXT NOT NULL DEFAULT '',
    bpm             INTEGER NOT NULL DEFAULT 0,
    meter           TEXT NOT NULL DEFAULT '',
    length_seconds  INTEGER NOT NULL DEFAULT 0,
    sequence        TEXT NOT NULL DEFAULT '',
    notes           TEXT NOT NULL DEFAULT '',
    original_key    TEXT NOT NULL DEFAULT '',
    source_content  TEXT NOT NULL DEFAULT '',
    format          TEXT NOT NULL DEFAULT '',
    lyrics          TEXT NOT NULL DEFAULT '',
    chords          TEXT NOT NULL DEFAULT '',
    archived        BOOLEAN NOT NULL DEFAULT FALSE,
    revision_note   TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX song_arrangement_revisions_arr_idx
    ON song_arrangement_revisions (arrangement_id, created_at DESC);

CREATE TABLE practice_tokens (
    token           TEXT PRIMARY KEY,
    song_id         UUID NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
    arrangement_id  UUID REFERENCES song_arrangements(id) ON DELETE SET NULL,
    display_key     TEXT NOT NULL DEFAULT '',
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked         BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX practice_tokens_song_idx    ON practice_tokens (song_id);
CREATE INDEX practice_tokens_expires_idx ON practice_tokens (expires_at) WHERE NOT revoked;

-- +goose Down
DROP TABLE IF EXISTS practice_tokens;
DROP TABLE IF EXISTS song_arrangement_revisions;
