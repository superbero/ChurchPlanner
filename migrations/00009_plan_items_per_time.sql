-- +goose Up
ALTER TABLE plan_items
    ADD COLUMN plan_time_id UUID REFERENCES plan_times(id) ON DELETE CASCADE;

-- Backfill: attach each existing item to the plan's "service" time, or the
-- earliest time if there's no service time.
UPDATE plan_items pi SET plan_time_id = COALESCE(
    (SELECT pt.id FROM plan_times pt
     WHERE pt.plan_id = pi.plan_id AND pt.kind = 'service'
     ORDER BY pt.starts_at LIMIT 1),
    (SELECT pt.id FROM plan_times pt
     WHERE pt.plan_id = pi.plan_id
     ORDER BY pt.starts_at LIMIT 1)
);

CREATE INDEX plan_items_plan_time_idx ON plan_items (plan_time_id, position);

-- +goose Down
DROP INDEX IF EXISTS plan_items_plan_time_idx;
ALTER TABLE plan_items DROP COLUMN IF EXISTS plan_time_id;
