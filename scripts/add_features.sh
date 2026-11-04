#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  Cleaning up the Navigation Bar"
echo "════════════════════════════════════════════════════════════════"

python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/web/templates/layout.html")
src = p.read_text()

# Find the entire <nav> block and replace it
start = src.find('<nav class="top-nav">')
end = src.find('</nav>') + len('</nav>')

if start == -1 or end == -1:
    print("  ✗ Could not find <nav> block")
    exit(1)

new_nav = '''<nav class="top-nav">
        <div class="nav-container">
            <a href="/" class="nav-brand">Church Planner</a>

            <div class="nav-links">
                {{if .CurrentUser}}
                    {{if eq .CurrentUser.Role "admin"}}
                        <a href="/plans">Plans</a>
                        <a href="/people">People</a>
                        <a href="/teams">Teams</a>
                        <a href="/songs">Songs</a>
                        <a href="/calendar">Calendar</a>
                        <a href="/reports/ccli">Reports</a>
                        <a href="/checkin" target="_blank" class="nav-checkin" title="Open Kiosk in New Tab">Kiosk ↗</a>
                    {{else if eq .CurrentUser.Role "manager"}}
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

src = src[:start] + new_nav + src[end:]
p.write_text(src)
print("  ✓ Navigation bar cleaned up")
PYEOF

# Bump cache
python3 - <<'PYEOF'
from pathlib import Path
import re
p = Path("internal/web/templates/layout.html")
src = p.read_text()
m = re.search(r'app\.(css|js)\?v=(\d+)', src)
if m:
    new_v = int(m.group(2)) + 1
    src = src.replace(f'app.css?v={m.group(2)}', f'app.css?v={new_v}')
    src = src.replace(f'app.js?v={m.group(2)}', f'app.js?v={new_v}')
    p.write_text(src)
PYEOF

if ! go build ./...; then
    echo "✗ build failed — paste errors"
    exit 1
fi
echo "✓ build OK"

docker compose -f deploy/docker-compose.yml up -d --build
