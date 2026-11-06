-- +goose Up
CREATE TABLE service_templates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_type_id UUID NOT NULL REFERENCES service_types(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (service_type_id, name)
);

CREATE TABLE service_template_times (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id          UUID NOT NULL REFERENCES service_templates(id) ON DELETE CASCADE,
    position             INTEGER NOT NULL,
    kind                 TEXT NOT NULL CHECK (kind IN ('service', 'rehearsal', 'other')),
    name                 TEXT NOT NULL,
    start_offset_minutes INTEGER NOT NULL DEFAULT 0,
    duration_minutes     INTEGER NOT NULL DEFAULT 0,
    UNIQUE (template_id, position)
);

-- +goose Down
DROP TABLE service_template_times;
DROP TABLE service_templates;
