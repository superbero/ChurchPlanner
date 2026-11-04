#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  Fixing Kebab Menu Stacking Issue"
echo "════════════════════════════════════════════════════════════════"

echo "▸ [1/3] Rewriting kebab menu CSS cleanly"
python3 - <<'PYEOF'
from pathlib import Path
import re

p = Path("internal/web/static/css/app.css")
src = p.read_text()

# Remove any previously added kebab/table fixes to avoid conflicts
# Remove the block we added earlier
for marker in [
    '/* Fix table corners since we removed overflow: hidden */',
    '/* ── Kebab Menu (Three Dots) ───────────────────────────────────── */',
]:
    if marker in src:
        # Find the end of this block (next blank line followed by new comment, or EOF)
        start = src.find(marker)
        # Simple approach: remove up to the next triple newline or end of relevant section
        # For simplicity, let's just remove everything from marker to end and re-add
        if marker == '/* Fix table corners since we removed overflow: hidden */':
            src = src[:start]
        elif marker == '/* ── Kebab Menu (Three Dots) ───────────────────────────────────── */':
            # Just remove the kebab CSS block
            end_marker = '/* '
            next_idx = src.find('\n\n/* ', start + len(marker))
            if next_idx != -1:
                src = src[:start] + src[next_idx:]
            else:
                src = src[:start]

# Ensure table-container is correct
src = re.sub(
    r'\.table-container\s*\{[^}]*\}',
    '''.table-container {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    overflow: visible;
    box-shadow: var(--shadow-sm);
}''',
    src,
    count=1
)

# Add fresh, corrected kebab menu CSS
src += '''

/* ── Kebab Menu (Three Dots) ───────────────────────────────────── */
/* Ensure table rows don't clip dropdowns */
.table-container tr {
    position: static;
    z-index: auto;
}

/* Only the row with an open menu gets elevated */
.table-container tr.menu-open {
    position: relative;
    z-index: 100;
}

.kebab-menu {
    position: relative;
    display: inline-block;
}

.kebab-btn {
    list-style: none;
    cursor: pointer;
    padding: 0.35rem 0.6rem;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    font-size: 1.2rem;
    font-weight: bold;
    border: 1px solid transparent;
    user-select: none;
}
.kebab-btn::-webkit-details-marker {
    display: none;
}
.kebab-btn:hover {
    background: var(--bg-hover);
    border-color: var(--border);
    color: var(--text-main);
}

.kebab-dropdown {
    position: absolute;
    right: 0;
    top: 100%;
    margin-top: 0.25rem;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    box-shadow: 0 10px 25px rgba(0,0,0,0.15), 0 4px 6px rgba(0,0,0,0.05);
    min-width: 160px;
    z-index: 9999;
    padding: 0.5rem 0;
}

.kebab-dropdown a,
.kebab-dropdown button {
    display: block;
    width: 100%;
    text-align: left;
    padding: 0.5rem 1rem;
    font: inherit;
    font-size: 0.9rem;
    color: var(--text-main);
    background: transparent;
    border: 0;
    text-decoration: none;
    cursor: pointer;
}
.kebab-dropdown a:hover,
.kebab-dropdown button:hover {
    background: var(--bg-hover);
    text-decoration: none;
}
.kebab-divider {
    height: 1px;
    background: var(--border);
    margin: 0.5rem 0;
}
.kebab-dropdown .kebab-danger {
    color: var(--danger);
}
.kebab-dropdown .kebab-danger:hover {
    background: var(--danger-bg);
}

/* Table corner rounding (replacing overflow: hidden) */
.table-container thead tr:first-child th:first-child {
    border-top-left-radius: var(--radius-md);
}
.table-container thead tr:first-child th:last-child {
    border-top-right-radius: var(--radius-md);
}
.table-container tbody tr:last-child td:first-child {
    border-bottom-left-radius: var(--radius-md);
}
.table-container tbody tr:last-child td:last-child {
    border-bottom-right-radius: var(--radius-md);
}
'''
p.write_text(src)
print("  ✓ CSS completely rewritten for kebab menu")
PYEOF

echo "▸ [2/3] Adding JS to toggle menu-open class"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/web/static/js/app.js")
src = p.read_text()

if '// Kebab menu row elevation' not in src:
    src += '''

// Kebab menu row elevation
// Prevents table rows from painting over the dropdown menu
(function () {
    document.addEventListener('toggle', function (e) {
        if (!e.target.matches('details.kebab-menu')) return;

        const tr = e.target.closest('tr');
        if (!tr) return;

        if (e.target.open) {
            // Close all other open menus first
            document.querySelectorAll('details.kebab-menu[open]').forEach(d => {
                if (d !== e.target) d.open = false;
            });
            tr.classList.add('menu-open');
        } else {
            tr.classList.remove('menu-open');
        }
    }, true); // useCapture=true because toggle doesn't bubble

    // Close menu when clicking outside
    document.addEventListener('click', function (e) {
        if (!e.target.closest('details.kebab-menu')) {
            document.querySelectorAll('details.kebab-menu[open]').forEach(d => {
                d.open = false;
            });
        }
    });
})();
'''
    p.write_text(src)
    print("  ✓ JS toggle logic added")
else:
    print("  · JS already present")
PYEOF

echo "▸ [3/3] Bumping cache & building"
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
