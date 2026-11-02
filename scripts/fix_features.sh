#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "▸ 1/3  Move /checkin out of the auth group (make it public)"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/web/router.go")
src = p.read_text()

old = '\t\tcheckin.New(d.DB, renderer).Routes(r)\n'
if old not in src:
    if '\tcheckin.New(d.DB, renderer).Routes(r)\n' in src:
        print("  · checkin already public")
        raise SystemExit(0)
    print("  ! checkin route mount not found")
    raise SystemExit(1)

src = src.replace(old, '', 1)

marker = '\tsongsModule.PublicRoutes(r)'
if marker not in src:
    print("  ! PublicRoutes marker not found")
    raise SystemExit(1)

addition = '''\tsongsModule.PublicRoutes(r)

\t// Kiosk check-in: intentionally public. Parents walk up to a lobby
\t// tablet and check in kids without a staff login.
\tcheckin.New(d.DB, renderer).Routes(r)'''

src = src.replace(marker, addition, 1)
p.write_text(src)
print("  ✓ /checkin is now public")
PYEOF

echo ""
echo "▸ 2/3  Add Check-in link to the nav bar"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/web/templates/layout.html")
src = p.read_text()

if 'href="/checkin"' in src:
    print("  · already present")
    raise SystemExit(0)

# Add "Check-in" right before the closing {{end}} of {{if .CurrentUser}}
marker = '''                {{end}}
            {{end}}'''

replacement = '''                {{end}}
                <a href="/checkin" target="_blank" title="Opens kiosk in a new tab">Check-in</a>
            {{end}}'''

if marker in src:
    src = src.replace(marker, replacement, 1)
    p.write_text(src)
    print("  ✓ Check-in link added to nav")
else:
    # Fallback: add after Reports for admin
    fb = '<a href="/reports/ccli">Reports</a>'
    if fb in src:
        src = src.replace(fb, fb + '\n                    <a href="/checkin" target="_blank">Check-in</a>', 1)
        p.write_text(src)
        print("  ✓ Check-in link added (fallback)")
    else:
        print("  ! could not find nav marker")
PYEOF

# Bump CSS/JS cache so layout changes take effect
python3 - <<'PYEOF'
from pathlib import Path
import re
p = Path("internal/web/templates/layout.html")
src = p.read_text()
for asset in ("app.css", "app.js"):
    pat = rf'{re.escape(asset)}\?v=(\d+)'
    m = re.search(pat, src)
    if m:
        new_v = int(m.group(1)) + 1
        src = src.replace(m.group(0), f'{asset}?v={new_v}')
p.write_text(src)
print("  ✓ cache bumped")
PYEOF

echo ""
echo "▸ 3/3  Build & restart"
if ! go build ./...; then
    echo "✗ build failed — paste errors"
    exit 1
fi
echo "✓ build OK"

docker compose -f deploy/docker-compose.yml up -d --build

echo ""
echo "▸ Logs (15s)"
timeout 15 docker compose -f deploy/docker-compose.yml logs -f app 2>&1 || true

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  Verify"
echo "════════════════════════════════════════════════════════════════"
cat <<'CHECKLIST'
  1. Open /checkin in a fresh incognito window.
     → Should load the kiosk form WITHOUT redirecting to /login.

  2. Logged in as admin:
     → Nav bar now shows a "Check-in" link (opens in new tab).

  3. Try the full flow:
     → Enter 555-0199 → family screen → check a kid in → see QR code.

  4. Log out (POST /logout), then hit /checkin again.
     → Should still load. No more 303 redirect to /login.
CHECKLIST
