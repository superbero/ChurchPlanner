-- +goose Up
CREATE TABLE smtp_settings (
    id                       SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    host                     TEXT NOT NULL,
    port                     INTEGER NOT NULL,
    username                 TEXT NOT NULL DEFAULT '',
    password_encrypted       BYTEA NOT NULL,
    from_address             TEXT NOT NULL,
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE smtp_settings;
