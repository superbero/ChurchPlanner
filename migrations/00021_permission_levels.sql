-- +goose Up
UPDATE users
SET role = CASE role
    WHEN 'admin' THEN 'admin'
    WHEN 'manager' THEN 'manager'
    WHEN 'editor' THEN 'editor'
    WHEN 'viewer' THEN 'viewer'
    WHEN 'guest' THEN 'viewer'
    WHEN 'scheduled_viewer' THEN 'scheduled_viewer'
    WHEN 'volunteer' THEN 'scheduled_viewer'
    ELSE 'scheduled_viewer'
END;

ALTER TABLE users ALTER COLUMN role SET DEFAULT 'scheduled_viewer';
ALTER TABLE users
    ADD CONSTRAINT users_role_check
    CHECK (role IN ('admin', 'manager', 'editor', 'viewer', 'scheduled_viewer'));

-- +goose Down
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
UPDATE users
SET role = CASE role
    WHEN 'admin' THEN 'admin'
    WHEN 'manager' THEN 'volunteer'
    WHEN 'editor' THEN 'editor'
    WHEN 'viewer' THEN 'guest'
    ELSE 'volunteer'
END;
ALTER TABLE users ALTER COLUMN role SET DEFAULT 'volunteer';
