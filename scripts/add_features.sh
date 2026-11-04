#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f go.mod ]]; then
    echo "Run this script from the ChurchPlanner project root." >&2
    exit 1
fi

if [[ ! -f migrations/00024_user_title.sql ]]; then
    echo "Missing user-title migration: migrations/00024_user_title.sql" >&2
    exit 1
fi

echo "Running Go tests and static analysis..."
go test ./...
go vet ./...

echo "Checks passed. The application applies database migrations on startup."
