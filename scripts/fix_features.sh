#!/usr/bin/env bash
# Diagnostic + apply missing fixes. Explicitly disables -e so no step aborts.
set +e

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  Diagnostic: what's already applied?"
echo "════════════════════════════════════════════════════════════════"

NEED2A=0; NEED2B=0; NEED3A=0; NEED3B=0; NEED4=0

if grep -q 'u.Role == "admin" || u.Role == "editor"' internal/scheduling/handler.go 2>/dev/null; then
    echo "  ✓ [2a] canManagePosition allows editor"
else
    echo "  ✗ [2a] canManagePosition does NOT allow editor"
    NEED2A=1
fi

if grep -q 'u.Role != "admin" && u.Role != "editor"' internal/scheduling/handler.go 2>/dev/null; then
    echo "  ✓ [2b] Plan mutation handlers allow editor"
else
    echo "  ✗ [2b] Plan mutation handlers do NOT allow editor"
    NEED2B=1
fi

if grep -q 'u.Role == "admin" || u.Role == "editor"' internal/web/router.go 2>/dev/null; then
    echo "  ✓ [3a] /plans middleware allows editor"
else
    echo "  ✗ [3a] /plans middleware does NOT allow editor"
    NEED3A=1
fi

if grep -q 'if u.Role == "admin" || u.Role == "editor" {' internal/web/router.go 2>/dev/null; then
    echo "  ✓ [3b] Dashboard redirect handles editor"
else
    echo "  ✗ [3b] Dashboard redirect does NOT handle editor"
    NEED3B=1
fi

if grep -q 'value="editor"' internal/people/templates/form.html 2>/dev/null; then
    echo "  ✓ [4]  New Person form has all 4 roles"
else
    echo "  ✗ [4]  New Person form missing roles"
    NEED4=1
fi

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  Applying missing fixes"
echo "════════════════════════════════════════════════════════════════"

if [ "$NEED2A" = "1" ] || [ "$NEED2B" = "1" ]; then
    echo ""
    echo "▸ [2/4] Patch scheduling/handler.go"
    python3 <<'PYEOF' 2>&1 | sed 's/^/    /'
from pathlib import Path
p = Path("internal/scheduling/handler.go")
src = p.read_text()
changed = 0

# 2a — canManagePosition
old1 = '\tif u.Role == "admin" {\n\t\treturn true, nil\n\t}'
new1 = '\tif u.Role == "admin" || u.Role == "editor" {\n\t\treturn true, nil\n\t}'
if old1 in src:
    src = src.replace(old1, new1, 1)
    changed += 1
    print("✓ canManagePosition now allows editor")

# 2b — admin-required plan mutation handlers
old2 = '\tif u, ok := auth.UserFrom(r.Context()); !ok || u.Role != "admin" {\n\t\thttp.Error(w, "admin required", http.StatusForbidden)\n\t\treturn\n\t}'
new2 = '\tif u, ok := auth.UserFrom(r.Context()); !ok || (u.Role != "admin" && u.Role != "editor") {\n\t\thttp.Error(w, "admin or editor required", http.StatusForbidden)\n\t\treturn\n\t}'
n = src.count(old2)
if n > 0:
    src = src.replace(old2, new2)
    changed += n
    print(f"✓ Updated {n} admin-required block(s)")
elif 'admin or editor required' in src:
    print("· Plan mutation handlers already updated")

p.write_text(src)
if changed == 0:
    print("· Nothing needed")
PYEOF
fi

if [ "$NEED3A" = "1" ] || [ "$NEED3B" = "1" ]; then
    echo ""
    echo "▸ [3/4] Patch web/router.go"
    python3 <<'PYEOF' 2>&1 | sed 's/^/    /'
from pathlib import Path
p = Path("internal/web/router.go")
src = p.read_text()
changed = 0

# 3a — teamLeaderOrAdmin middleware
old1 = '\t\t\tif u.Role == "admin" {\n\t\t\t\tnext.ServeHTTP(w, r)\n\t\t\t\treturn\n\t\t\t}'
new1 = '\t\t\tif u.Role == "admin" || u.Role == "editor" {\n\t\t\t\tnext.ServeHTTP(w, r)\n\t\t\t\treturn\n\t\t\t}'
if old1 in src:
    src = src.replace(old1, new1, 1)
    changed += 1
    print("✓ /plans middleware allows editor")

# 3b — dashboard redirect
old2 = '\t\t\tif u.Role == "admin" {\n\t\t\t\thttp.Redirect(w, r, "/plans", http.StatusSeeOther)\n\t\t\t\treturn\n\t\t\t}'
new2 = '\t\t\tif u.Role == "admin" || u.Role == "editor" {\n\t\t\t\thttp.Redirect(w, r, "/plans", http.StatusSeeOther)\n\t\t\t\treturn\n\t\t\t}'
if old2 in src:
    src = src.replace(old2, new2, 1)
    changed += 1
    print("✓ Dashboard redirect handles editor")

p.write_text(src)
if changed == 0:
    print("· Nothing needed")
PYEOF
fi

if [ "$NEED4" = "1" ]; then
    echo ""
    echo "▸ [4/4] Patch people/templates/form.html"
    python3 <<'PYEOF' 2>&1 | sed 's/^/    /'
from pathlib import Path
p = Path("internal/people/templates/form.html")
src = p.read_text()

old = '''        <select name="role">
            <option value="volunteer" {{if eq .Role "volunteer"}}selected{{end}}>Volunteer</option>
            <option value="admin" {{if eq .Role "admin"}}selected{{end}}>Admin</option>
        </select>'''
new = '''        <select name="role">
            <option value="volunteer" {{if eq .Role "volunteer"}}selected{{end}}>Volunteer (Scheduled Viewer)</option>
            <option value="guest"     {{if eq .Role "guest"}}selected{{end}}>Guest</option>
            <option value="editor"    {{if eq .Role "editor"}}selected{{end}}>Editor</option>
            <option value="admin"     {{if eq .Role "admin"}}selected{{end}}>Admin</option>
        </select>'''

if old in src:
    p.write_text(src.replace(old, new, 1))
    print("✓ Form now has all 4 roles")
else:
    print("! Could not match current select block")
    print("  Open internal/people/templates/form.html and look for:")
    print("    <select name=\"role\">")
PYEOF
fi

# ═══════════════════════════════════════════════════════════════════
echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  Build"
echo "════════════════════════════════════════════════════════════════"

go build ./... 2>&1
BUILD_STATUS=$?

if [ $BUILD_STATUS -ne 0 ]; then
    echo ""
    echo "✗ Build failed — see errors above"
    exit 1
fi
echo "✓ build OK"

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  Restart"
echo "════════════════════════════════════════════════════════════════"
docker compose -f deploy/docker-compose.yml up -d --build

echo ""
echo "▸ Logs (15s)"
timeout 15 docker compose -f deploy/docker-compose.yml logs -f app 2>&1 || true

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  Verify"
echo "════════════════════════════════════════════════════════════════"
cat <<'CHECKLIST'
  1. /people → "Add new person" → dropdown shows 4 roles
  2. Log in as Jane (Editor) → lands on /plans
  3. As Jane: add song, assign, publish, delete → all work
  4. As Jane: /people → 403
  5. As Jane: no People/Reports in nav
CHECKLIST
