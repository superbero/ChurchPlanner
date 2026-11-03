#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  Applying Professional Layout to Core Pages"
echo "════════════════════════════════════════════════════════════════"

echo "▸ [1/3] Polishing Songs List"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/songs/templates/list.html")
src = p.read_text()

# Update the empty state
src = src.replace(
    '<tr><td colspan="3" style="text-align: center; padding: 2rem;">No songs found. Try clearing your filters or adding a new song.</td></tr>',
    '<tr><td colspan="6" style="text-align: center; padding: 3rem; color: var(--text-muted);">No songs found. Try clearing your filters or adding a new song.</td></tr>'
)

# Update the actions cell to use clean links
old_actions = '''                <td class="actions-cell">
                    <button class="button-secondary" onclick="showPreview('{{.ID}}')">Quick View</button>
                    <a href="/songs/{{.ID}}" style="margin-left: 0.5rem;">Details</a> ·
                    <a href="#" onclick="showPdfModal('{{.ID}}', '{{.Title}}', event); return false;">PDF</a>
                    {{if isEditorOrAdmin $.CurrentUser.Role}}· <a href="/songs/{{.ID}}/edit">Edit</a>{{end}}
                </td>'''
new_actions = '''                <td class="actions-cell">
                    <div class="action-group">
                        <button class="button-secondary btn-sm" onclick="showPreview('{{.ID}}')">Quick View</button>
                        <a href="/songs/{{.ID}}" class="action-link">Details</a>
                        <a href="#" onclick="showPdfModal('{{.ID}}', '{{.Title}}', event); return false;" class="action-link">PDF</a>
                        {{if isEditorOrAdmin $.CurrentUser.Role}}
                        <a href="/songs/{{.ID}}/edit" class="action-link">Edit</a>
                        {{end}}
                    </div>
                </td>'''
src = src.replace(old_actions, new_actions)
p.write_text(src)
print("  ✓ Songs list polished")
PYEOF

echo "▸ [2/3] Polishing Plans List"
cat > internal/scheduling/templates/plans.html <<'HTMLEOF'
{{define "content"}}
<div class="page-header">
    <div>
        <h1>Plans</h1>
        <p class="muted">Schedule services and manage team assignments.</p>
    </div>
    <div class="action-bar">
        <a class="button" href="/plans/new">New Plan</a>
        <a class="button-secondary" href="/my-schedule">My Schedule</a>
    </div>
</div>

<form method="get" action="/plans" class="filter-bar-horizontal">
    <div class="filter-group">
        <select name="type" style="min-width: 200px;">
            <option value="">All Service Types</option>
            {{range .ServiceTypes}}
            <option value="{{.ID}}" {{if eq .ID $.SelectedType}}selected{{end}}>{{.Name}}</option>
            {{end}}
        </select>
    </div>
    <div class="filter-group" style="margin-left: auto;">
        <button type="submit" class="button-secondary">Filter</button>
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
                    <div class="action-group">
                        <a href="/plans/{{.ID}}" class="button-secondary btn-sm">Open Plan</a>
                        <a href="/plans/{{.ID}}/edit" class="action-link">Edit</a>
                    </div>
                </td>
            </tr>
        {{else}}
            <tr><td colspan="4" style="text-align: center; padding: 3rem; color: var(--text-muted);">No plans found. Click "New Plan" to get started.</td></tr>
        {{end}}
        </tbody>
    </table>
</div>
{{end}}
HTMLEOF
echo "  ✓ Plans list polished"

echo "▸ [3/3] Polishing People List"
cat > internal/people/templates/list.html <<'HTMLEOF'
{{define "content"}}
<div class="page-header">
    <div>
        <h1>People</h1>
        <p class="muted">Directory of volunteers and staff.</p>
    </div>
    <div class="action-bar">
        <a class="button" href="/people/new">Add Person</a>
    </div>
</div>

<div class="table-container">
    <table>
        <thead>
            <tr>
                <th>Name</th>
                <th>Contact</th>
                <th>Team(s)</th>
                <th>Skills</th>
                <th style="text-align:right">Actions</th>
            </tr>
        </thead>
        <tbody>
        {{range .Users}}
            <tr>
                <td>
                    <a href="/people/{{.ID}}" style="font-weight: 600; font-size: 1.05em; color: var(--text-main);">{{.Name}}</a>
                    <div style="margin-top: 0.25rem;">
                        <span class="chip">{{.Role}}</span>
                    </div>
                </td>
                <td>
                    <div style="font-size: 0.9em;">{{.Email}}</div>
                    {{if .Phone}}<div class="muted" style="font-size: 0.85em;">{{.Phone}}</div>{{end}}
                </td>
                <td class="team-cell">
                    {{if .TeamRoles}}
                        <div style="display: flex; flex-wrap: wrap; gap: 0.25rem;">
                        {{range split .TeamRoles ", "}}
                            <span class="chip" style="background: var(--brand-50); color: var(--brand-600);">{{.}}</span>
                        {{end}}
                        </div>
                    {{else}}
                        <span class="muted">—</span>
                    {{end}}
                </td>
                <td class="skills-cell">
                    {{if .Skills}}
                        <div style="font-size: 0.85em; color: var(--text-muted); line-height: 1.4;">{{.Skills}}</div>
                    {{else}}
                        <span class="muted">—</span>
                    {{end}}
                </td>
                <td class="actions-cell">
                    <div class="action-group">
                        <a href="/people/{{.ID}}" class="button-secondary btn-sm">Edit Profile</a>
                    </div>
                </td>
            </tr>
        {{else}}
            <tr><td colspan="5" style="text-align: center; padding: 3rem; color: var(--text-muted);">No people found.</td></tr>
        {{end}}
        </tbody>
    </table>
</div>
{{end}}
HTMLEOF
echo "  ✓ People list polished"

echo "▸ Appending final UI helper CSS"
cat >> internal/web/static/css/app.css <<'CSSEOF'

/* ── Professional Table Helpers ────────────────────────────────── */
.actions-cell { text-align: right; white-space: nowrap; }
.action-group { display: flex; justify-content: flex-end; gap: 0.75rem; align-items: center; }
.action-link { font-size: 0.9em; color: var(--text-muted); text-decoration: none; }
.action-link:hover { color: var(--text-main); text-decoration: underline; }
.btn-sm { padding: 0.35rem 0.75rem; font-size: 0.85rem; }
.team-cell, .skills-cell { max-width: 250px; vertical-align: top; }
CSSEOF

# Bump cache
python3 - <<'PYEOF'
from pathlib import Path
import re
p = Path("internal/web/templates/layout.html")
src = p.read_text()
m = re.search(r'app\.css\?v=(\d+)', src)
if m:
    src = src.replace(m.group(0), f'app.css?v={int(m.group(1))+1}')
    p.write_text(src)
PYEOF

if ! go build ./...; then
    echo "✗ build failed — paste errors"
    exit 1
fi
echo "✓ build OK"

docker compose -f deploy/docker-compose.yml up -d --build
