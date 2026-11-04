-- +goose Up
-- Using ON CONFLICT to safely ignore if they were already inserted by a previous run.
INSERT INTO service_types (name, position) VALUES
    ('Cohesion', 3),
    ('Training Day', 4),
    ('Soirée de Louange', 5)
ON CONFLICT (name) DO NOTHING;

-- +goose Down
DELETE FROM service_types WHERE name IN ('Cohesion', 'Training Day', 'Soirée de Louange');
