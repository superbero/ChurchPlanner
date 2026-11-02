-- +goose Up

-- 1. New tables
CREATE TABLE song_arrangements (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    song_id         UUID NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    bpm             INTEGER NOT NULL DEFAULT 0,
    meter           TEXT NOT NULL DEFAULT '',
    length_seconds  INTEGER NOT NULL DEFAULT 0,
    sequence        TEXT NOT NULL DEFAULT '',
    notes           TEXT NOT NULL DEFAULT '',
    original_key    TEXT NOT NULL DEFAULT '',
    source_content  TEXT NOT NULL DEFAULT '',
    format          TEXT NOT NULL DEFAULT 'chordpro',
    lyrics          TEXT NOT NULL DEFAULT '',
    chords          TEXT NOT NULL DEFAULT '',
    is_default      BOOLEAN NOT NULL DEFAULT FALSE,
    archived        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (song_id, name)
);

CREATE INDEX song_arrangements_song_idx ON song_arrangements (song_id);
CREATE UNIQUE INDEX song_arrangements_default_uniq
    ON song_arrangements (song_id) WHERE is_default = TRUE;

CREATE TABLE arrangement_keys (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    arrangement_id  UUID NOT NULL REFERENCES song_arrangements(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    key             TEXT NOT NULL,
    is_default      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (arrangement_id, name)
);

CREATE INDEX arrangement_keys_arrangement_idx ON arrangement_keys (arrangement_id);
CREATE UNIQUE INDEX arrangement_keys_default_uniq
    ON arrangement_keys (arrangement_id) WHERE is_default = TRUE;

-- 2. Backfill: one "Original" arrangement per song
INSERT INTO song_arrangements (
    song_id, name, bpm, meter, length_seconds,
    sequence, notes, original_key, source_content, format,
    lyrics, chords, is_default
)
SELECT
    id, 'Original',
    COALESCE(tempo, 0),
    COALESCE(meter, ''),
    COALESCE(length_seconds, 0),
    '',
    COALESCE(notes, ''),
    COALESCE(NULLIF(original_key, ''), NULLIF(key, ''), ''),
    COALESCE(source_content, ''),
    COALESCE(NULLIF(format, ''), 'manual'),
    COALESCE(lyrics, ''),
    COALESCE(chords, ''),
    TRUE
FROM songs;

-- 3. Backfill: one "Original" key per arrangement that has a key value
INSERT INTO arrangement_keys (arrangement_id, name, key, is_default)
SELECT id, 'Original', original_key, TRUE
FROM song_arrangements
WHERE original_key <> '';

-- 4. Attach arrangement to key history (nullable)
ALTER TABLE song_key_history
    ADD COLUMN arrangement_id UUID REFERENCES song_arrangements(id) ON DELETE SET NULL;

-- 5. Replace songs search trigger (songs can't reference lyrics anymore)
DROP TRIGGER IF EXISTS songs_search_vector_trigger ON songs;
DROP FUNCTION IF EXISTS songs_search_vector_update();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION songs_search_vector_update() RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.title, '')),  'A') ||
        setweight(to_tsvector('english', COALESCE(NEW.author, '')), 'B');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER songs_search_vector_trigger
    BEFORE INSERT OR UPDATE ON songs
    FOR EACH ROW EXECUTE FUNCTION songs_search_vector_update();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION arrangements_search_vector_update() RETURNS TRIGGER AS $$
DECLARE
    target_song_id UUID;
BEGIN
    target_song_id := COALESCE(NEW.song_id, OLD.song_id);
    UPDATE songs SET search_vector =
        setweight(to_tsvector('english', COALESCE(title, '')),  'A') ||
        setweight(to_tsvector('english', COALESCE(author, '')), 'B') ||
        setweight(to_tsvector('english', COALESCE(
            (SELECT string_agg(strip_chords(lyrics), ' ')
             FROM song_arrangements
             WHERE song_id = songs.id AND NOT archived), '')
        ), 'C')
    WHERE id = target_song_id;
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER arrangements_search_vector_trigger
    AFTER INSERT OR UPDATE OR DELETE ON song_arrangements
    FOR EACH ROW EXECUTE FUNCTION arrangements_search_vector_update();

-- 6. Drop old musical columns from songs
ALTER TABLE songs
    DROP COLUMN IF EXISTS key,
    DROP COLUMN IF EXISTS original_key,
    DROP COLUMN IF EXISTS tempo,
    DROP COLUMN IF EXISTS meter,
    DROP COLUMN IF EXISTS length_seconds,
    DROP COLUMN IF EXISTS lyrics,
    DROP COLUMN IF EXISTS chords,
    DROP COLUMN IF EXISTS source_content,
    DROP COLUMN IF EXISTS format;

-- 7. Refresh search vectors with new aggregates
UPDATE songs SET title = title;

-- +goose Down
DROP TRIGGER IF EXISTS arrangements_search_vector_trigger ON song_arrangements;
DROP FUNCTION IF EXISTS arrangements_search_vector_update();

ALTER TABLE songs
    ADD COLUMN key             TEXT NOT NULL DEFAULT '',
    ADD COLUMN original_key    TEXT NOT NULL DEFAULT '',
    ADD COLUMN tempo           INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN meter           TEXT NOT NULL DEFAULT '',
    ADD COLUMN length_seconds  INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN lyrics          TEXT NOT NULL DEFAULT '',
    ADD COLUMN chords          TEXT NOT NULL DEFAULT '',
    ADD COLUMN source_content  TEXT NOT NULL DEFAULT '',
    ADD COLUMN format          TEXT NOT NULL DEFAULT 'manual';

UPDATE songs s SET
    key            = COALESCE((SELECT a.original_key FROM song_arrangements a WHERE a.song_id = s.id AND a.is_default LIMIT 1), ''),
    original_key   = COALESCE((SELECT a.original_key FROM song_arrangements a WHERE a.song_id = s.id AND a.is_default LIMIT 1), ''),
    tempo          = COALESCE((SELECT a.bpm          FROM song_arrangements a WHERE a.song_id = s.id AND a.is_default LIMIT 1), 0),
    meter          = COALESCE((SELECT a.meter        FROM song_arrangements a WHERE a.song_id = s.id AND a.is_default LIMIT 1), ''),
    length_seconds = COALESCE((SELECT a.length_seconds FROM song_arrangements a WHERE a.song_id = s.id AND a.is_default LIMIT 1), 0),
    lyrics         = COALESCE((SELECT a.lyrics       FROM song_arrangements a WHERE a.song_id = s.id AND a.is_default LIMIT 1), ''),
    chords         = COALESCE((SELECT a.chords       FROM song_arrangements a WHERE a.song_id = s.id AND a.is_default LIMIT 1), ''),
    source_content = COALESCE((SELECT a.source_content FROM song_arrangements a WHERE a.song_id = s.id AND a.is_default LIMIT 1), ''),
    format         = COALESCE((SELECT a.format       FROM song_arrangements a WHERE a.song_id = s.id AND a.is_default LIMIT 1), 'manual');

DROP TABLE IF EXISTS arrangement_keys;
DROP TABLE IF EXISTS song_arrangements;
ALTER TABLE song_key_history DROP COLUMN IF EXISTS arrangement_id;
