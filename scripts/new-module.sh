#!/usr/bin/env bash
# Usage: ./scripts/new-module.sh events
set -euo pipefail
mod="${1:?module name required}"
mkdir -p "internal/$mod/templates"
echo "Created internal/$mod/. Next steps:"
echo "  1. Add model.go, repository.go, service.go, handler.go"
echo "  2. Copy routes.go from internal/songs/routes.go and adapt"
echo "  3. Add template partials under internal/$mod/templates/*.html"
echo "  4. Register in internal/web/router.go (renderer.Add + New(...).Routes(r))"
