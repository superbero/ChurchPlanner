-- +goose Up
CREATE TABLE children (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_phone TEXT NOT NULL,
    first_name   TEXT NOT NULL,
    last_name    TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX children_phone_idx ON children (parent_phone);

CREATE TABLE checkins (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    child_id        UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    security_code   TEXT NOT NULL,
    checked_in_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    checked_out_at  TIMESTAMPTZ
);
CREATE INDEX checkins_child_idx ON checkins (child_id, checked_in_at DESC);

-- +goose Down
DROP TABLE IF EXISTS checkins;
DROP TABLE IF EXISTS children;
