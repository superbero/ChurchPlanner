#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  Converting Plan Actions to an Accordion Menu"
echo "════════════════════════════════════════════════════════════════"

echo "▸ [1/3] Rewriting plans.html with Accordion Rows"
cat > internal/scheduling/templates/plans.html <<'HTMLEOF'
{{define "content"}}
<div class="page-header">
    <div>
        <h1>Plans</h1>
        <p class="muted">Schedule services and manage team assignments.</p>
    </div>
    <div class="action-bar">
        <a class="button" href="/plans/new">New Plan</a>
        <a class="button-secondary" href="/plans/bulk">Bulk Create</a>
        <a class="button-secondary" href="/my-schedule">My Schedule</a>
    </div>
</div>

<form method="get" action="/plans" class="filter-bar-horizontal">
    <div class="filter-group">
        <select name="type" style="min-width: 150px;">
            <option value="">All Service Types</option>
            {{range .ServiceTypes}}
            <option value="{{.ID}}" {{if eq .ID $.SelectedType}}selected{{end}}>{{.Name}}</option>
            {{end}}
        </select>
    </div>

    <div class="filter-group">
        <select name="date_range" style="min-width: 140px;">
            <option value="upcoming" {{if eq .DateRange "upcoming"}}selected{{end}}>Upcoming</option>
            <option value="this_month" {{if eq .DateRange "this_month"}}selected{{end}}>This Month</option>
            <option value="last_month" {{if eq .DateRange "last_month"}}selected{{end}}>Last Month</option>
            <option value="past" {{if eq .DateRange "past"}}selected{{end}}>Past Plans</option>
        </select>
    </div>

    <div class="filter-group">
        <select name="limit" style="min-width: 100px;">
            <option value="10" {{if eq .Limit 10}}selected{{end}}>Show 10</option>
            <option value="25" {{if eq .Limit 25}}selected{{end}}>Show 25</option>
            <option value="50" {{if eq .Limit 50}}selected{{end}}>Show 50</option>
        </select>
    </div>

    <div class="filter-group" style="margin-left: auto;">
        <button type="submit" class="button-secondary">Apply Filters</button>
        <a href="/plans" class="button-secondary">Clear</a>
    </div>
</form>

{{if .TypeName}}<p class="muted" style="margin-bottom: 1rem;">Showing: <strong>{{.TypeName}}</strong></p>{{end}}

<div class="table-container">
    <table>
        <thead>
            <tr>
                <th>Date</th>
                <th>Service</th>
                <th>Status</th>
                <th style="text-align:right">Actions</th>
            </tr>
        </thead>
        <tbody>
        {{range .Plans}}
            <!-- Main Row -->
            <tr>
                <td style="white-space: nowrap;">
                    <strong>{{.ServiceAt.Format "Mon, Jan 2"}}</strong>
                    <span class="muted" style="margin-left: 0.5rem;">{{.ServiceAt.Format "3:04 PM"}}</span>
                </td>
                <td>
                    <a href="/plans/{{.ID}}" style="font-weight: 500; font-size: 1.05em;">{{.Title}}</a>
                </td>
                <td>
                    {{if .Published}}
                        <span class="chip" style="background: var(--success-bg); color: var(--success);">Published</span>
                    {{else}}
                        <span class="chip" style="background: var(--warning-bg); color: var(--warning);">Draft</span>
                    {{end}}
                </td>
                <td class="actions-cell">
                    <button type="button" class="button-secondary btn-sm toggle-actions" data-target="actions-{{.ID}}">
                        Manage ▼
                    </button>
                </td>
            </tr>
            <!-- Expandable Action Row -->
            <tr id="actions-{{.ID}}" class="accordion-row" style="display: none;">
                <td colspan="4">
                    <div class="action-toolbar">
                        <a href="/plans/{{.ID}}" class="button-secondary btn-sm">Open Plan</a>
                        <a href="/plans/{{.ID}}/edit" class="button-secondary btn-sm">Edit Details</a>

                        <form method="post" action="/plans/{{.ID}}/publish" style="margin:0;">
                            <input type="hidden" name="published" value="{{if .Published}}0{{else}}1{{end}}">
                            <button type="submit" class="button-secondary btn-sm">{{if .Published}}Unpublish{{else}}Publish{{end}}</button>
                        </form>

                        <div style="flex:1;"></div> <!-- Spacer -->

                        <form method="post" action="/plans/{{.ID}}/delete" onsubmit="return confirm('Delete this plan and everything in it?')" style="margin:0;">
                            <button type="submit" class="button-danger btn-sm">Delete Plan</button>
                        </form>
                    </div>
                </td>
            </tr>
        {{else}}
            <tr><td colspan="4" style="text-align: center; padding: 3rem; color: var(--text-muted);">No plans found matching your filters.</td></tr>
        {{end}}
        </tbody>
    </table>
</div>

<script>
    document.addEventListener('DOMContentLoaded', function() {
        const toggleButtons = document.querySelectorAll('.toggle-actions');

        toggleButtons.forEach(btn => {
            btn.addEventListener('click', function() {
                const targetId = this.getAttribute('data-target');
                const targetRow = document.getElementById(targetId);

                if (targetRow.style.display === 'none') {
                    // Close all others first
                    document.querySelectorAll('.accordion-row').forEach(row => {
                        row.style.display = 'none';
                    });
                    document.querySelectorAll('.toggle-actions').forEach(b => {
                        b.innerHTML = 'Manage ▼';
                    });

                    // Open this one
                    targetRow.style.display = 'table-row';
                    this.innerHTML = 'Close ▲';
                } else {
                    targetRow.style.display = 'none';
                    this.innerHTML = 'Manage ▼';
                }
            });
        });
    });
</script>
{{end}}
HTMLEOF
echo "  ✓ plans.html rewritten"

echo "▸ [2/3] Updating CSS for Accordion"
python3 - <<'PYEOF'
from pathlib import Path
import re

p = Path("internal/web/static/css/app.css")
src = p.read_text()

# Remove old kebab CSS
start = src.find('/* ── Kebab Menu (Three Dots) ───────────────────────────────────── */')
if start != -1:
    src = src[:start]

# Add Accordion CSS
src += '''

/* ── Accordion Action Rows ─────────────────────────────────────── */
.accordion-row td {
    background: var(--bg-body);
    padding: 0; /* Remove padding to let toolbar span */
    border-bottom: 1px solid var(--border);
}

.action-toolbar {
    display: flex;
    gap: 0.75rem;
    padding: 1rem 1.5rem;
    align-items: center;
}

.action-toolbar form {
    margin: 0;
}

.btn-sm {
    padding: 0.35rem 0.75rem;
    font-size: 0.85rem;
}

.toggle-actions {
    min-width: 100px;
}
'''
p.write_text(src)
print("  ✓ CSS updated for Accordion")
PYEOF

echo "▸ [3/3] Build & Restart"
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
