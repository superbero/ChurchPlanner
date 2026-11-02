-- +goose Up
-- Which positions are actually needed for a specific plan.
-- PCO calls this "positions" within a plan; we keep the global `positions`
-- table as the catalog, and this table records which ones this plan uses.
CREATE TABLE plan_positions (
    plan_id     UUID NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
    position_id UUID NOT NULL REFERENCES positions(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (plan_id, position_id)
);

CREATE INDEX plan_positions_plan_idx ON plan_positions (plan_id);

-- +goose Down
DROP TABLE IF EXISTS plan_positions;
