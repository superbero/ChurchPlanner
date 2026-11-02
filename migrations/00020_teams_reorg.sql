-- +goose Up
-- Rename the two legacy teams to match the church's naming
UPDATE teams SET name = 'Worship Team' WHERE name = 'Worship';
UPDATE teams SET name = 'Media Team'   WHERE name = 'Production';

-- Ensure Sound exists as its own team
INSERT INTO teams (name) VALUES ('Sound')
ON CONFLICT (name) DO NOTHING;

-- Move the existing "Sound" position from Media Team to the Sound team
UPDATE positions
SET team_id = (SELECT id FROM teams WHERE name = 'Sound')
WHERE name = 'Sound'
  AND team_id = (SELECT id FROM teams WHERE name = 'Media Team');

-- +goose Down
UPDATE positions
SET team_id = (SELECT id FROM teams WHERE name = 'Media Team')
WHERE name = 'Sound'
  AND team_id = (SELECT id FROM teams WHERE name = 'Sound');
DELETE FROM teams WHERE name = 'Sound';
UPDATE teams SET name = 'Worship'    WHERE name = 'Worship Team';
UPDATE teams SET name = 'Production' WHERE name = 'Media Team';
