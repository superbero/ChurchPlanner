-- +goose Up
CREATE TABLE team_memberships (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    team_id    UUID NOT NULL REFERENCES teams(id)  ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'member',  -- 'member' | 'leader'
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, team_id)
);
CREATE INDEX team_memberships_user_idx ON team_memberships (user_id);
CREATE INDEX team_memberships_team_role_idx ON team_memberships (team_id, role);

-- +goose Down
DROP TABLE IF EXISTS team_memberships;
