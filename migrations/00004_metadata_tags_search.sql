-- +goose Up
ALTER TABLE songs
    ADD COLUMN ccli_number      TEXT    NOT NULL DEFAULT '',
    ADD COLUMN copyright_holder TEXT    NOT NULL DEFAULT '',
    ADD COLUMN copyright_date   TEXT    NOT NULL DEFAULT '',
    ADD COLUMN publisher        TEXT    NOT NULL DEFAULT '',
    ADD COLUMN meter            TEXT    NOT NULL DEFAULT '',
    ADD COLUMN length_seconds   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN scripture        TEXT    NOT NULL DEFAULT '',
    ADD COLUMN notes            TEXT    NOT NULL DEFAULT '',
    ADD COLUMN archived         BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN search_vector    TSVECTOR;

CREATE UNIQUE INDEX songs_ccli_number_uniq
    ON songs (ccli_number) WHERE ccli_number <> '';

CREATE INDEX songs_archived_idx ON songs (archived);
CREATE INDEX songs_key_idx      ON songs (key);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION strip_chords(t TEXT) RETURNS TEXT AS $$
BEGIN
    RETURN regexp_replace(COALESCE(t, ''), '\[[^]]*\]', '', 'g');
END;
$$ LANGUAGE plpgsql IMMUTABLE;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION songs_search_vector_update() RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.title, '')),  'A') ||
        setweight(to_tsvector('english', COALESCE(NEW.author, '')), 'B') ||
        setweight(to_tsvector('english', strip_chords(NEW.lyrics)), 'C');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER songs_search_vector_trigger
    BEFORE INSERT OR UPDATE ON songs
    FOR EACH ROW EXECUTE FUNCTION songs_search_vector_update();

UPDATE songs SET title = title;

CREATE INDEX songs_search_idx ON songs USING GIN (search_vector);

CREATE TABLE tag_groups (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL UNIQUE,
    position   INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tags (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id   UUID NOT NULL REFERENCES tag_groups(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (group_id, slug)
);

CREATE INDEX tags_slug_idx  ON tags (slug);
CREATE INDEX tags_group_idx ON tags (group_id);

CREATE TABLE song_tags (
    song_id UUID NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
    tag_id  UUID NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
    PRIMARY KEY (song_id, tag_id)
);

CREATE INDEX song_tags_tag_idx ON song_tags (tag_id);

INSERT INTO tag_groups (name, position) VALUES
    ('Theme',  1),
    ('Season', 2),
    ('Moment', 3),
    ('Tempo',  4)
ON CONFLICT (name) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS song_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS tag_groups;
DROP TRIGGER IF EXISTS songs_search_vector_trigger ON songs;
DROP FUNCTION IF EXISTS songs_search_vector_update();
DROP FUNCTION IF EXISTS strip_chords(TEXT);
DROP INDEX IF EXISTS songs_search_idx;
DROP INDEX IF EXISTS songs_key_idx;
DROP INDEX IF EXISTS songs_archived_idx;
DROP INDEX IF EXISTS songs_ccli_number_uniq;
ALTER TABLE songs
    DROP COLUMN IF EXISTS search_vector,
    DROP COLUMN IF EXISTS archived,
    DROP COLUMN IF EXISTS notes,
    DROP COLUMN IF EXISTS scripture,
    DROP COLUMN IF EXISTS length_seconds,
    DROP COLUMN IF EXISTS meter,
    DROP COLUMN IF EXISTS publisher,
    DROP COLUMN IF EXISTS copyright_date,
    DROP COLUMN IF EXISTS copyright_holder,
    DROP COLUMN IF EXISTS ccli_number;
