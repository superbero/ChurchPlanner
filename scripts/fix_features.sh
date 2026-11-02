#!/usr/bin/env bash
set -euo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "▸ 1/3  Rewriting migration 00018 with StatementBegin/End"
cat > migrations/00018_guardians.sql <<'SQLEOF'
-- +goose Up
CREATE TABLE families (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE guardians (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    family_id  UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    name       TEXT NOT NULL DEFAULT '',
    phone      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX guardians_phone_idx ON guardians (phone);

ALTER TABLE children ADD COLUMN family_id UUID REFERENCES families(id) ON DELETE CASCADE;

-- +goose StatementBegin
DO $$
DECLARE
    r RECORD;
    new_family_id UUID;
BEGIN
    FOR r IN SELECT DISTINCT parent_phone FROM children WHERE parent_phone <> '' LOOP
        INSERT INTO families DEFAULT VALUES RETURNING id INTO new_family_id;
        INSERT INTO guardians (family_id, phone) VALUES (new_family_id, r.parent_phone);
        UPDATE children SET family_id = new_family_id WHERE parent_phone = r.parent_phone;
    END LOOP;
END $$;
-- +goose StatementEnd

ALTER TABLE children DROP COLUMN parent_phone;

-- +goose Down
ALTER TABLE children ADD COLUMN parent_phone TEXT NOT NULL DEFAULT '';
DROP TABLE IF EXISTS guardians;
DROP TABLE IF EXISTS families;
SQLEOF

echo "  ✓ migration rewritten with StatementBegin/End"

echo ""
echo "▸ 2/3  Build & Restart"
if ! go build ./...; then
    echo "✗ build failed"
    exit 1
fi
echo "✓ build OK"

docker compose -f deploy/docker-compose.yml up -d --build

echo ""
echo "▸ 3/3  Tailing app logs (15s)"
timeout 15 docker compose -f deploy/docker-compose.yml logs -f app 2>&1 || true

echo ""
echo "✅ Done."
echo "If logs show 'listening', the migration applied cleanly."
