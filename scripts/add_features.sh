#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  Restoring Add Position Form & People Serving Layout"
echo "════════════════════════════════════════════════════════════════"

python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/scheduling/templates/plan_view.html")
src = p.read_text()

# Find the People Serving section and replace it completely
start_marker = '<!-- BOTTOM: People Serving Tabs -->'
end_marker = '<!-- Song Picker Modal -->'

start_idx = src.find(start_marker)
end_idx = src.find(end_marker)

if start_idx == -1 or end_idx == -1:
    print("  ✗ Could not find the People Serving section boundaries")
    exit(1)

new_section = '''<!-- BOTTOM: People Serving Tabs -->
<div class="people-serving-section">
    <div class="section-header">
        <h2>People Serving</h2>
    </div>

    {{if .Teams}}
    <div class="tabs">
        <!-- Tab Buttons -->
        <div class="tab-buttons" role="tablist">
            {{range $i, $team := .Teams}}
            <button class="tab-btn {{if eq $i 0}}active{{end}}" data-target="team-{{$team.TeamID}}" role="tab">
                {{$team.TeamName}}
            </button>
            {{end}}
        </div>

        <!-- Tab Panels -->
        <div class="tab-content">
            {{range $i, $team := .Teams}}
            <div id="team-{{$team.TeamID}}" class="tab-panel {{if eq $i 0}}active{{end}}" role="tabpanel">
                <div class="team-card">
                    <div class="position-list">
                        {{range $team.Positions}}
                        <div class="position-row">
                            <div class="position-info">
                                <strong>{{.PositionName}}</strong>
                                {{if .Assignees}}
                                    {{range .Assignees}}
                                    <div class="assignee-status">
                                        <span class="status-dot status-{{.Status}}"></span>
                                        <span>{{.PersonName}}</span>
                                    </div>
                                    {{end}}
                                {{else}}
                                    <span class="status-badge status-open">Open</span>
                                {{end}}
                            </div>

                            <div class="position-actions">
                                {{if index $.ManageablePositionIDs .PositionID}}
                                <form method="post" action="/plans/{{$.Plan.ID}}/people" class="assign-form">
                                    <input type="hidden" name="position_id" value="{{.PositionID}}">
                                    <select name="person_id" required onchange="this.form.submit()">
                                        <option value="">Assign…</option>
                                        {{$eligible := index $.UsersByPos .PositionID}}
                                        {{range $eligible}}
                                        <option value="{{.ID}}">{{.Name}}{{if index $.BlockedUsers .ID}} 🚫{{end}}</option>
                                        {{end}}
                                    </select>
                                </form>
                                {{end}}

                                {{if .Assignees}}
                                    {{range .Assignees}}
                                    <form method="post" action="/plans/{{$.Plan.ID}}/people/{{.PlanPersonID}}/delete" style="display:inline;">
                                        <button class="link-btn" type="submit" style="color: var(--danger);" title="Remove">×</button>
                                    </form>
                                    {{end}}
                                {{end}}
                            </div>
                        </div>
                        {{else}}
                        <div class="empty-items" style="padding: 2rem;">No positions added to this team yet.</div>
                        {{end}}
                    </div>
                </div>
            </div>
            {{end}}
        </div>
    </div>
    {{else}}
        <div class="empty-state">
            <p>No teams configured. Add teams in the database to start scheduling.</p>
        </div>
    {{end}}

    {{if .ManageablePositions}}
    <div class="add-position-card" style="max-width: 100%; margin-top: 1.5rem;">
        <h3 style="margin-top: 0;">Add a Position to this Plan</h3>
        <form method="post" action="/plans/{{.Plan.ID}}/positions" class="inline-form" style="display: flex; gap: 0.5rem; align-items: center;">
            <select name="position_id" required style="flex: 1; max-width: 400px;">
                <option value="">— Select a position to add —</option>
                {{range .ManageablePositions}}
                <option value="{{.ID}}">{{.TeamName}} · {{.Name}}</option>
                {{end}}
            </select>
            <button type="submit" class="button">Add Position</button>
        </form>
    </div>
    {{end}}
</div>

'''

src = src[:start_idx] + new_section + src[end_idx:]
p.write_text(src)
print("  ✓ People Serving section restored with Add Position form")
PYEOF

echo "▸ Build & Restart"
if ! go build ./...; then
    echo "✗ build failed — paste errors"
    exit 1
fi
echo "✓ build OK"

docker compose -f deploy/docker-compose.yml up -d --build
