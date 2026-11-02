-- +goose Up
ALTER TABLE song_arrangements
    ADD COLUMN primary_language TEXT NOT NULL DEFAULT 'en';

CREATE TABLE arrangement_translations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    arrangement_id  UUID NOT NULL REFERENCES song_arrangements(id) ON DELETE CASCADE,
    language        TEXT NOT NULL,
    lyrics          TEXT NOT NULL DEFAULT '',
    chords          TEXT NOT NULL DEFAULT '',
    source_content  TEXT NOT NULL DEFAULT '',
    format          TEXT NOT NULL DEFAULT 'chordpro',
    notes           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (arrangement_id, language)
);

CREATE INDEX arrangement_translations_arr_idx ON arrangement_translations (arrangement_id);

-- +goose Down
DROP TABLE IF EXISTS arrangement_translations;
ALTER TABLE song_arrangements DROP COLUMN IF EXISTS primary_language;
