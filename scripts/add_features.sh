#!/usr/bin/env bash
set -euo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

LAYOUT="internal/web/templates/layout.html"
[ -f "$LAYOUT" ] || { echo "✗ missing $LAYOUT"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  Cleaning up the Navigation Bar"
echo "════════════════════════════════════════════════════════════════"

python3 - "$LAYOUT" <<'PYEOF'
import re, sys
from pathlib import Path

p = Path(sys.argv[1])
src = p.read_text()

OPEN = '<nav class="top-nav">'
CLOSE = '</nav>'

start = src.find(OPEN)
if start == -1:
    print(f'  ✗ Could not find {OPEN!r} block', file=sys.stderr)
    sys.exit(1)

# Find the matching </nav> AFTER start — not the first one in the file.
end = src.find(CLOSE, start)
if end == -1:
    print(f'  ✗ Could not find closing {CLOSE!r} after {OPEN!r}', file=sys.stderr)
    sys.exit(1)
end += len(CLOSE)

new_nav = '''<nav class="top-nav">
        <div class="nav-container">
            <a href="/" class="nav-brand">Church Planner</a>

            <div class="nav-links">
                {{if .CurrentUser}}
                    {{if or (eq .CurrentUser.Role "admin") (eq .CurrentUser.Role "manager")}}
                        <a href="/plans">Plans</a>
                        <a href="/people">People</a>
                        <a href="/teams">Teams</a>
                        <a href="/songs">Songs</a>
                        <a href="/calendar">Calendar</a>
                        <a href="/reports/ccli">Reports</a>
                        <a href="/checkin" target="_blank" class="nav-checkin" title="Open Kiosk in New Tab">Kiosk ↗</a>
                    {{else if eq .CurrentUser.Role "editor"}}
                        <a href="/plans">Plans</a>
                        <a href="/songs">Songs</a>
                        <a href="/calendar">Calendar</a>
                    {{else}}
                        <a href="/my-schedule">My Schedule</a>
                        <a href="/calendar">Calendar</a>
                        <a href="/songs">Songs</a>
                    {{end}}
                {{end}}
            </div>

            {{if .CurrentUser}}
            <div class="nav-user">
                <div class="user-info">
                    <span class="user-name">{{.CurrentUser.Name}}</span>
                    <span class="user-role">{{.CurrentUser.Role}}</span>
                </div>
                <form method="post" action="/logout" class="nav-logout">
                    <button type="submit">Sign out</button>
                </form>
            </div>
            {{end}}
        </div>
    </nav>'''

current = src[start:end]
if current == new_nav:
    print("  • Navigation bar already up to date — skipping rewrite")
else:
    src = src[:start] + new_nav + src[end:]
    print("  ✓ Navigation bar cleaned up")

# Cache-bump: handle CSS and JS independently, once each.
def bump(m):
    return f"{m.group(1)}{int(m.group(2)) + 1}"

new_src, n = re.subn(r'(app\.(?:css|js)\?v=)(\d+)', bump, src)
if n:
    print(f"  ✓ Bumped {n} asset cache version(s)")
    src = new_src
else:
    print("  • No app.css/app.js ?v= found — nothing to bump")

if src != p.read_text():
    p.write_text(src)
PYEOF

if ! go build ./...; then
    echo "✗ build failed — paste errors"
    exit 1
fi
echo "✓ build OK"

if command -v docker >/dev/null 2>&1; then
    docker compose -f deploy/docker-compose.yml up -d --build
else
    echo "• docker not found — skipping compose up"
fi
