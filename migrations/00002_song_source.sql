-- +goose Up
ALTER TABLE songs
    ADD COLUMN format TEXT NOT NULL DEFAULT 'manual',
    ADD COLUMN source_content TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE songs
    DROP COLUMN source_content,
    DROP COLUMN format;
