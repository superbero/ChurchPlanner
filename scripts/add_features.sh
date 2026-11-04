#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  Implementing Bulk Plan Creation (Recurring Plans)"
echo "════════════════════════════════════════════════════════════════"

echo "▸ [1/5] Adding BulkCreatePlans to scheduling/handler.go"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/scheduling/handler.go")
src = p.read_text()

if 'func (h *Handler) BulkCreatePlans' not in src:
    src += '''

func (h *Handler) ShowBulkPlan(w http.ResponseWriter, r *http.Request) {
	types, err := h.svcTypes.List(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	h.renderer.Render(w, r, http.StatusOK, "scheduling_plan_bulk", map[string]any{
		"ServiceTypes": types,
	})
}

func (h *Handler) BulkCreatePlans(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpx.ServerError(w, err)
		return
	}

	typeID := r.FormValue("service_type_id")
	titleBase := strings.TrimSpace(r.FormValue("title_base"))
	startStr := r.FormValue("start_date")
	endStr := r.FormValue("end_date")
	frequency := r.FormValue("frequency")

	startDate, _ := time.Parse("2006-01-02", startStr)
	endDate, _ := time.Parse("2006-01-02", endStr)

	if startDate.IsZero() || endDate.IsZero() || endDate.Before(startDate) {
		http.Error(w, "Invalid date range", http.StatusBadRequest)
		return
	}

	// Default times: 10:00 AM - 11:30 AM
	startTime := 10
	startMinute := 0
	endTime := 11
	endMinute := 30

	created := 0
	current := startDate

	for !current.After(endDate) {
		// 1. Check if a plan already exists on this date for this service type
		var exists bool
		h.plans.DB().QueryRow(r.Context(), `
			SELECT EXISTS(
				SELECT 1 FROM plans
				WHERE service_type_id = $1 AND service_at::date = $2::date
			)
		`, typeID, current.Format("2006-01-02")).Scan(&exists)

		if !exists {
			serviceAt := time.Date(current.Year(), current.Month(), current.Day(), startTime, startMinute, 0, 0, time.Local)
			serviceEnds := time.Date(current.Year(), current.Month(), current.Day(), endTime, endMinute, 0, 0, time.Local)

			// Insert Plan
			var planID string
			err := h.plans.DB().QueryRow(r.Context(), `
				INSERT INTO plans (service_type_id, title, service_at, service_ends_at)
				VALUES ($1, $2, $3, $4)
				RETURNING id
			`, typeID, titleBase+" - "+current.Format("Jan 2"), serviceAt, serviceEnds).Scan(&planID)

			if err == nil {
				// Create the default Service Time block
				h.plans.CreateTime(r.Context(), planID, "service", "Service", serviceAt, serviceEnds)
				created++
			}
		}

		// 2. Increment date based on frequency
		switch frequency {
		case "weekly":
			current = current.AddDate(0, 0, 7)
		case "biweekly":
			current = current.AddDate(0, 0, 14)
		case "monthly":
			current = current.AddDate(0, 1, 0)
		default:
			current = current.AddDate(0, 0, 7) // fallback to weekly
		}
	}

	http.Redirect(w, r, "/plans", http.StatusSeeOther)
}
'''
    p.write_text(src)
    print("  ✓ Bulk handlers added")

echo "▸ [2/5] Adding routes"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/scheduling/routes.go")
src = p.read_text()

if '/plans/bulk' not in src:
    src = src.replace(
        'r.Get("/new", m.handler.NewPlan)',
        'r.Get("/new", m.handler.NewPlan)\n\t\tr.Get("/bulk", m.handler.ShowBulkPlan)\n\t\tr.Post("/bulk", m.handler.BulkCreatePlans)'
    )
    p.write_text(src)
    print("  ✓ Routes added")
PYEOF

echo "▸ [3/5] Creating scheduling_plan_bulk template"
cat > internal/scheduling/templates/plan_bulk.html <<'HTMLEOF'
{{define "content"}}
<div class="page-header">
    <div>
        <p class="breadcrumb"><a href="/plans">← All plans</a></p>
        <h1>Bulk Create Plans</h1>
    </div>
</div>

<div class="card" style="max-width: 640px;">
    <p class="muted" style="margin-top: 0; margin-bottom: 1.5rem;">
        Generate multiple plans at once for predictable schedules (e.g., every Sunday for the next 3 months).
    </p>

    <form method="post" action="/plans/bulk" id="bulk-form">
        <label>Service type
            <select name="service_type_id" required>
                {{range .ServiceTypes}}
                <option value="{{.ID}}">{{.Name}}</option>
                {{end}}
            </select>
        </label>

        <label>Title Base
            <input name="title_base" required placeholder="e.g. Sunday Service" value="Sunday Service">
            <span class="muted" style="font-size: 0.8em; font-weight: normal; display: block; margin-top: 0.25rem;">
                We will append the date, e.g. "Sunday Service - Nov 8".
            </span>
        </label>

        <div class="row">
            <label>Start Date
                <input type="date" name="start_date" id="start_date" required>
            </label>
            <label>End Date
                <input type="date" name="end_date" id="end_date" required>
            </label>
        </div>

        <label>Frequency
            <select name="frequency">
                <option value="weekly">Every Week</option>
                <option value="biweekly">Every 2 Weeks</option>
                <option value="monthly">Every Month</option>
            </select>
        </label>

        <div id="preview-box" style="background: var(--bg-body); padding: 1rem; border-radius: var(--radius-md); margin-top: 1.5rem; display: none;">
            <strong style="display: block; margin-bottom: 0.5rem;">Preview</strong>
            <p class="muted" style="font-size: 0.9em; margin: 0;">You are about to generate <strong id="preview-count">0</strong> plans.</p>
        </div>

        <div style="margin-top: 1.5rem; display: flex; gap: 0.75rem;">
            <button type="submit" class="button" onclick="return confirm('Generate these plans?')">Generate Plans</button>
            <a href="/plans" class="button-secondary">Cancel</a>
        </div>
    </form>
</div>

<script>
    document.addEventListener('DOMContentLoaded', function() {
        const start = document.getElementById('start_date');
        const end = document.getElementById('end_date');
        const freq = document.querySelector('select[name="frequency"]');
        const box = document.getElementById('preview-box');
        const countEl = document.getElementById('preview-count');

        function updatePreview() {
            if (!start.value || !end.value) { box.style.display = 'none'; return; }

            const sDate = new Date(start.value);
            const eDate = new Date(end.value);
            if (eDate < sDate) { box.style.display = 'none'; return; }

            let count = 0;
            let curr = new Date(sDate);

            while (curr <= eDate && count < 100) { // cap preview at 100
                count++;
                if (freq.value === 'weekly') curr.setDate(curr.getDate() + 7);
                else if (freq.value === 'biweekly') curr.setDate(curr.getDate() + 14);
                else if (freq.value === 'monthly') curr.setMonth(curr.getMonth() + 1);
                else curr.setDate(curr.getDate() + 7);
            }

            countEl.textContent = count;
            box.style.display = 'block';
        }

        start.addEventListener('change', updatePreview);
        end.addEventListener('change', updatePreview);
        freq.addEventListener('change', updatePreview);
    });
</script>
{{end}}
HTMLEOF
echo "  ✓ Template created"

echo "▸ [4/5] Adding Bulk Create button to plans.html"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/scheduling/templates/plans.html")
src = p.read_text()

old_actions = '''    <div class="action-bar">
        <a class="button" href="/plans/new">New Plan</a>
        <a class="button-secondary" href="/my-schedule">My Schedule</a>
    </div>'''

new_actions = '''    <div class="action-bar">
        <a class="button" href="/plans/new">New Plan</a>
        <a class="button-secondary" href="/plans/bulk">Bulk Create</a>
        <a class="button-secondary" href="/my-schedule">My Schedule</a>
    </div>'''

if old_actions in src:
    p.write_text(src.replace(old_actions, new_actions, 1))
    print("  ✓ Button added")
else:
    print("  ! Could not find action bar")
PYEOF

echo "▸ [5/5] Build & Restart"
if ! go build ./...; then
    echo "✗ build failed — paste errors"
    exit 1
fi
echo "✓ build OK"

docker compose -f deploy/docker-compose.yml up -d --build
