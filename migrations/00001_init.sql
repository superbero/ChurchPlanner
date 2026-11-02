-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'volunteer',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    token  TEXT PRIMARY KEY,
    data   BYTEA NOT NULL,
    expiry TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions (expiry);

CREATE TABLE songs (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title      TEXT NOT NULL,
    author     TEXT NOT NULL DEFAULT '',
    key        TEXT NOT NULL DEFAULT '',
    tempo      INTEGER NOT NULL DEFAULT 0,
    lyrics     TEXT NOT NULL DEFAULT '',
    chords     TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE service_plans (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title      TEXT NOT NULL,
    service_at TIMESTAMPTZ NOT NULL,
    notes      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE plan_items (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id          UUID NOT NULL REFERENCES service_plans(id) ON DELETE CASCADE,
    position         INTEGER NOT NULL,
    kind             TEXT NOT NULL,
    title            TEXT NOT NULL,
    song_id          UUID REFERENCES songs(id) ON DELETE SET NULL,
    duration_minutes INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE assignments (
    id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id UUID NOT NULL REFERENCES service_plans(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role    TEXT NOT NULL,
    status  TEXT NOT NULL DEFAULT 'pending',
    UNIQUE (plan_id, user_id, role)
);

CREATE TABLE events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title      TEXT NOT NULL,
    starts_at  TIMESTAMPTZ NOT NULL,
    ends_at    TIMESTAMPTZ NOT NULL,
    location   TEXT NOT NULL DEFAULT '',
    ministry   TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed an admin so you can log in immediately. CHANGE THE PASSWORD.
INSERT INTO users (name, email, password_hash, role)
VALUES ('Admin', 'onesimeking@gmail.com', crypt('changeme', gen_salt('bf')), 'admin')
ON CONFLICT (email) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS assignments;
DROP TABLE IF EXISTS plan_items;
DROP TABLE IF EXISTS service_plans;
DROP TABLE IF EXISTS songs;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
