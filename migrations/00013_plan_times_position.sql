-- +goose Up
ALTER TABLE plan_times ADD COLUMN position INTEGER NOT NULL DEFAULT 0;

-- Backfill position based on current start time so nothing moves initially.
WITH ordered AS (
  SELECT id, ROW_NUMBER() OVER (PARTITION BY plan_id ORDER BY starts_at) - 1 AS rn
  FROM plan_times
)
UPDATE plan_times pt
SET position = o.rn
FROM ordered o
WHERE pt.id = o.id;

-- +goose Down
ALTER TABLE plan_times DROP COLUMN IF EXISTS position;
