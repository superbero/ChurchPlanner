#!/usr/bin/env bash
set -euo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  1/6  Auth Middleware: Support multiple allowed roles"
echo "════════════════════════════════════════════════════════════════"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/platform/auth/middleware.go")
src = p.read_text()

# Replace RequireRole with RequireRoles
old_func = '''func (a *Authenticator) RequireRole(role string) func(http.Handler) http.Handler {
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

new_func = '''// RequireRoles ensures the logged-in user has one of the allowed roles.
// Admin always passes, regardless of the requested roles.
func (a *Authenticator) RequireRoles(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return a.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, _ := UserFrom(r.Context())
			if u.Role == "admin" {
				next.ServeHTTP(w, r)
				return
			}
			for _, role := range allowedRoles {
				if u.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, "forbidden", http.StatusForbidden)
		}))
	}
}'''

if old_func in src:
    p.write_text(src.replace(old_func, new_func, 1))
    print("  ✓ RequireRoles added")
else:
    print("  ! RequireRole not found")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  2/6  Renderer: Add isEditorOrAdmin helper"
echo "════════════════════════════════════════════════════════════════"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/platform/httpx/render.go")
src = p.read_text()

if "isEditorOrAdmin" not in src:
    # Add helper function before funcMap
    helper = '''
func isEditorOrAdmin(role string) bool {
	return role == "admin" || role == "editor"
}

func funcMap() template.FuncMap {'''
    src = src.replace("func funcMap() template.FuncMap {", helper, 1)

    # Add to FuncMap
    src = src.replace(
        '"languageLabel":   languageLabel,',
        '"languageLabel":   languageLabel,\n\t\t"isEditorOrAdmin": isEditorOrAdmin,'
    )
    p.write_text(src)
    print("  ✓ isEditorOrAdmin helper added")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  3/6  Router: Update middleware calls"
echo "════════════════════════════════════════════════════════════════"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/web/router.go")
src = p.read_text()

src = src.replace('r.Use(authn.RequireRole("admin"))', 'r.Use(authn.RequireRoles("admin"))')
p.write_text(src)
print("  ✓ router updated")

# Update Songs routes to allow editor
p = Path("internal/songs/routes.go")
src = p.read_text()
src = src.replace('r.Use(authn.RequireRole("admin"))', 'r.Use(authn.RequireRoles("admin", "editor"))')
p.write_text(src)
print("  ✓ songs routes updated")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  4/6  People Template: Add Editor and Guest options"
echo "════════════════════════════════════════════════════════════════"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/people/templates/view.html")
src = p.read_text()

old_opt = '''            <select name="role">
                <option value="volunteer" {{if eq .User.Role "volunteer"}}selected{{end}}>Volunteer</option>
                <option value="admin" {{if eq .User.Role "admin"}}selected{{end}}>Admin</option>
            </select>'''

new_opt = '''            <select name="role">
                <option value="volunteer" {{if eq .User.Role "volunteer"}}selected{{end}}>Volunteer (Scheduled Viewer)</option>
                <option value="guest" {{if eq .User.Role "guest"}}selected{{end}}>Guest</option>
                <option value="editor" {{if eq .User.Role "editor"}}selected{{end}}>Editor</option>
                <option value="admin" {{if eq .User.Role "admin"}}selected{{end}}>Admin</option>
            </select>'''

if old_opt in src:
    p.write_text(src.replace(old_opt, new_opt, 1))
    print("  ✓ People dropdown updated")
else:
    print("  ! Role select not found")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  5/6  Update Templates for the new roles"
echo "════════════════════════════════════════════════════════════════"
python3 - <<'PYEOF'
from pathlib import Path

# 1. Layout Nav Bar
p = Path("internal/web/templates/layout.html")
src = p.read_text()
old_nav = '''            {{if .CurrentUser}}
                {{if eq .CurrentUser.Role "admin"}}
                    <a href="/plans">Plans</a>
                    <a href="/people">People</a>
                    <a href="/songs">Songs</a>
                    <a href="/calendar">Calendar</a>
                    <a href="/reports/ccli">Reports</a>
                {{else}}
                    <a href="/my-schedule">My Schedule</a>
                    <a href="/calendar">Calendar</a>
                    <a href="/songs">Songs</a>
                {{end}}
            {{end}}'''
new_nav = '''            {{if .CurrentUser}}
                {{if eq .CurrentUser.Role "admin"}}
                    <a href="/plans">Plans</a>
                    <a href="/people">People</a>
                    <a href="/songs">Songs</a>
                    <a href="/calendar">Calendar</a>
                    <a href="/reports/ccli">Reports</a>
                {{else if eq .CurrentUser.Role "editor"}}
                    <a href="/songs">Songs</a>
                    <a href="/calendar">Calendar</a>
                {{else}}
                    <a href="/my-schedule">My Schedule</a>
                    <a href="/calendar">Calendar</a>
                    <a href="/songs">Songs</a>
                {{end}}
            {{end}}'''
if old_nav in src:
    src = src.replace(old_nav, new_nav, 1)
    p.write_text(src)
    print("  ✓ layout.html nav updated")

# 2. Songs List
p = Path("internal/songs/templates/list.html")
src = p.read_text()
src = src.replace('{{if eq $.CurrentUser.Role "admin"}}', '{{if isEditorOrAdmin .CurrentUser.Role}}')
src = src.replace('{{if eq .CurrentUser.Role "admin"}}', '{{if isEditorOrAdmin .CurrentUser.Role}}')
# Fix a specific spot where $. was used but . is fine
src = src.replace('{{if isEditorOrAdmin .CurrentUser.Role}}', '{{if isEditorOrAdmin $.CurrentUser.Role}}', 1)
p.write_text(src)

# 3. Songs Detail
p = Path("internal/songs/templates/detail.html")
src = p.read_text()
src = src.replace('{{if eq .CurrentUser.Role "admin"}}', '{{if isEditorOrAdmin .CurrentUser.Role}}')
p.write_text(src)

# 4. Arrangement View
p = Path("internal/songs/templates/arrangement_view.html")
src = p.read_text()
src = src.replace('{{if eq .CurrentUser.Role "admin"}}', '{{if isEditorOrAdmin .CurrentUser.Role}}')
src = src.replace('{{if eq $.CurrentUser.Role "admin"}}', '{{if isEditorOrAdmin $.CurrentUser.Role}}')
p.write_text(src)

print("  ✓ Song templates updated for Editors")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  6/6  Build & Restart"
echo "════════════════════════════════════════════════════════════════"
if ! go build ./...; then
    echo "✗ build failed — paste errors"
    exit 1
fi
echo "✓ build OK"

docker compose -f deploy/docker-compose.yml up -d --build
echo ""
echo "✅ Done."
