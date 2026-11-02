-- +goose Up

-- Drop placeholder tables created in 00001_init.sql. They were scaffolded
-- before the scheduling module existed and were never populated. The real
-- scheduling schema (plans, plan_items, plan_people) replaces them here.
DROP TABLE IF EXISTS plan_items CASCADE;
DROP TABLE IF EXISTS assignments CASCADE;
DROP TABLE IF EXISTS service_plans CASCADE;
DROP TABLE IF EXISTS events CASCADE;


CREATE TABLE service_types (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    position    INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE plans (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_type_id UUID NOT NULL REFERENCES service_types(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    service_at      TIMESTAMPTZ NOT NULL,
    notes           TEXT NOT NULL DEFAULT '',
    published       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX plans_service_type_idx ON plans (service_type_id, service_at DESC);

CREATE TABLE plan_times (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id     UUID NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL DEFAULT 'service',  -- service | rehearsal | other
    name        TEXT NOT NULL,
    starts_at   TIMESTAMPTZ NOT NULL,
    ends_at     TIMESTAMPTZ NOT NULL
);
CREATE INDEX plan_times_plan_idx ON plan_times (plan_id, starts_at);

CREATE TABLE teams (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE positions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id     UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    position    INTEGER NOT NULL DEFAULT 0,
    UNIQUE (team_id, name)
);

CREATE TABLE plan_people (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id       UUID NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
    person_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    position_id   UUID NOT NULL REFERENCES positions(id) ON DELETE CASCADE,
    status        TEXT NOT NULL DEFAULT 'U',   -- U = unconfirmed, C = confirmed, D = declined
    decline_note  TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plan_id, person_id, position_id)
);
CREATE INDEX plan_people_plan_idx   ON plan_people (plan_id);
CREATE INDEX plan_people_person_idx ON plan_people (person_id);

CREATE TABLE plan_items (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id         UUID NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
    position        INTEGER NOT NULL DEFAULT 0,
    kind            TEXT NOT NULL DEFAULT 'song',  -- song | header | note
    title           TEXT NOT NULL DEFAULT '',
    arrangement_id  UUID REFERENCES song_arrangements(id) ON DELETE SET NULL,
    key_id          UUID REFERENCES arrangement_keys(id)   ON DELETE SET NULL,
    key             TEXT NOT NULL DEFAULT '',
    duration_minutes INTEGER NOT NULL DEFAULT 0,
    notes           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX plan_items_plan_idx ON plan_items (plan_id, position);

-- Backfill: attach existing key history rows to nothing new, but add a plan_id
ALTER TABLE song_key_history
    ADD COLUMN plan_id UUID REFERENCES plans(id) ON DELETE SET NULL;

-- Seed default teams and service types so the UI isn't empty
INSERT INTO service_types (name, position) VALUES
    ('Sunday Morning', 1),
    ('Midweek', 2)
ON CONFLICT (name) DO NOTHING;

INSERT INTO teams (name) VALUES
    ('Worship'),
    ('Production'),
    ('Kids')
ON CONFLICT (name) DO NOTHING;

INSERT INTO positions (team_id, name, position)
SELECT t.id, p.name, p.pos
FROM teams t
CROSS JOIN LATERAL (VALUES
    ('Worship',  'Worship Leader', 1),
    ('Worship',  'Acoustic',       2),
    ('Worship',  'Electric',       3),
    ('Worship',  'Bass',           4),
    ('Worship',  'Drums',          5),
    ('Worship',  'Keys',           6),
    ('Production','Sound',         1),
    ('Production','Lights',        2),
    ('Production','Livestream',    3),
    ('Kids',     'Kids Lead',      1),
    ('Kids',     'Kids Helper',    2)
) AS p(team_name, name, pos)
WHERE t.name = p.team_name
ON CONFLICT (team_id, name) DO NOTHING;

-- +goose Down
ALTER TABLE song_key_history DROP COLUMN IF EXISTS plan_id;
DROP TABLE IF EXISTS plan_items;
DROP TABLE IF EXISTS plan_people;
DROP TABLE IF EXISTS positions;
DROP TABLE IF EXISTS teams;
DROP TABLE IF EXISTS plan_times;
DROP TABLE IF EXISTS plans;
DROP TABLE IF EXISTS service_types;
