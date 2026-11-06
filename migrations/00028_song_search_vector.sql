-- +goose Up
-- Keep lyric terms indexed when song metadata is edited as well as when an
-- arrangement changes.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION songs_search_vector_update() RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.title, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(NEW.author, '')), 'B') ||
        setweight(to_tsvector('english', COALESCE(
            (SELECT string_agg(strip_chords(lyrics), ' ')
             FROM song_arrangements
             WHERE song_id = NEW.id AND NOT archived), ''
        )), 'C');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

UPDATE songs SET title = title;

-- +goose Down
-- Restore the metadata-only behavior from migration 00005.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION songs_search_vector_update() RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.title, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(NEW.author, '')), 'B');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

UPDATE songs SET title = title;
