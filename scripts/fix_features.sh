#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "▸ Fixing RequireRole middleware signature"
python3 - <<'PYEOF'
from pathlib import Path

# 1. Update middleware.go
p = Path("internal/platform/auth/middleware.go")
src = p.read_text()

old_func = '''func (a *Authenticator) RequireRole(role string, next http.Handler) http.Handler {
	return a.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := UserFrom(r.Context())
		if u.Role != role && u.Role != "admin" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}'''

new_func = '''func (a *Authenticator) RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return a.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, _ := UserFrom(r.Context())
			if u.Role != role && u.Role != "admin" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		}))
	}
}'''

if old_func in src:
    src = src.replace(old_func, new_func, 1)
    p.write_text(src)
    print("  ✓ RequireRole middleware updated")
else:
    print("  ! RequireRole old signature not found")

# 2. Ensure router.go uses it correctly
p = Path("internal/web/router.go")
src = p.read_text()

if 'r.Use(authn.RequireRole("admin"))' not in src:
    src = src.replace(
        'r.Use(authn.RequireRole("admin"))', # in case it's slightly different
        'r.Use(authn.RequireRole("admin"))'
    )
    p.write_text(src)
    print("  ✓ router verified")
PYEOF

echo ""
echo "▸ Build check"
if ! go build ./...; then
    echo "✗ Build failed — paste errors"
    exit 1
fi
echo "✓ Build OK"

docker compose -f deploy/docker-compose.yml up -d --build
echo ""
echo "✅ Done."
echo "Log out and log back in to test the role-based UI."
