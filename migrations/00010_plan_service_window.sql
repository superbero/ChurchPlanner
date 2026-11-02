-- +goose Up
ALTER TABLE plans ADD COLUMN service_ends_at TIMESTAMPTZ;
UPDATE plans SET service_ends_at = service_at + interval '90 minutes';
ALTER TABLE plans ALTER COLUMN service_ends_at SET NOT NULL;

ALTER TABLE plans ADD COLUMN service_time_id UUID REFERENCES plan_times(id) ON DELETE SET NULL;

UPDATE plans p SET service_time_id = (
    SELECT pt.id FROM plan_times pt
    WHERE pt.plan_id = p.id AND pt.kind = 'service' AND pt.name = 'Service'
    ORDER BY pt.starts_at LIMIT 1
);

-- +goose Down
ALTER TABLE plans DROP COLUMN IF EXISTS service_time_id;
ALTER TABLE plans DROP COLUMN IF EXISTS service_ends_at;
