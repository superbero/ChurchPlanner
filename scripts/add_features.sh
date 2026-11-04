#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  Implementing Smart Date Calculation for Bulk Plans"
echo "════════════════════════════════════════════════════════════════"

echo "▸ [1/2] Rewriting plan_bulk.html with auto-calculator"
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
        Generate multiple plans at once for predictable schedules. The end date will calculate automatically based on the number of occurrences.
    </p>

    <form method="post" action="/plans/bulk" id="bulk-form">
        <label>Service type
            <select name="service_type_id" id="bulk-type" required>
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
                <input type="date" name="start_date" id="bulk-start" required>
            </label>
            <label>Frequency
                <select name="frequency" id="bulk-freq">
                    <option value="weekly">Every Week</option>
                    <option value="biweekly">Every 2 Weeks</option>
                    <option value="monthly">Every Month</option>
                </select>
            </label>
        </div>

        <div class="row" style="align-items: flex-end;">
            <label>Number of Occurrences
                <input type="number" id="bulk-occurrences" value="4" min="1" max="52" required>
            </label>
            <label>Calculated End Date
                <input type="date" name="end_date" id="bulk-end" readonly style="background: var(--bg-body); color: var(--text-muted);">
            </label>
        </div>

        <div id="preview-box" style="background: var(--bg-body); padding: 1rem; border-radius: var(--radius-md); margin-top: 1.5rem;">
            <strong style="display: block; margin-bottom: 0.5rem;">Summary</strong>
            <p class="muted" style="font-size: 0.9em; margin: 0;">
                Generating <strong id="preview-count">4</strong> plans starting on <strong id="preview-start">—</strong> and ending on <strong id="preview-end">—</strong>.
            </p>
        </div>

        <div style="margin-top: 1.5rem; display: flex; gap: 0.75rem;">
            <button type="submit" class="button" onclick="return confirm('Generate these plans?')">Generate Plans</button>
            <a href="/plans" class="button-secondary">Cancel</a>
        </div>
    </form>
</div>

<script>
    document.addEventListener('DOMContentLoaded', function() {
        const typeSelect = document.getElementById('bulk-type');
        const startInput = document.getElementById('bulk-start');
        const freqSelect = document.getElementById('bulk-freq');
        const occurrencesInput = document.getElementById('bulk-occurrences');
        const endInput = document.getElementById('bulk-end');

        const previewCount = document.getElementById('preview-count');
        const previewStart = document.getElementById('preview-start');
        const previewEnd = document.getElementById('preview-end');

        // Helper to format a date as YYYY-MM-DD
        function toYMD(date) {
            const yyyy = date.getFullYear();
            const mm = String(date.getMonth() + 1).padStart(2, '0');
            const dd = String(date.getDate()).padStart(2, '0');
            return `${yyyy}-${mm}-${dd}`;
        }

        // Helper to format a date for display (e.g. Nov 8, 2026)
        function toDisplay(date) {
            return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
        }

        // 1. Calculate start date based on Service Type
        function calculateStartDate() {
            const typeText = typeSelect.options[typeSelect.selectedIndex].text;
            let targetDay = -1;
            if (typeText.includes('Sunday')) targetDay = 0;
            else if (typeText.includes('Midweek') || typeText.includes('Wednesday')) targetDay = 3;
            else if (typeText.includes('Louange') || typeText.includes('Friday')) targetDay = 5;

            if (targetDay !== -1) {
                const today = new Date();
                let distance = targetDay - today.getDay();
                if (distance <= 0) distance += 7;

                const nextDate = new Date(today.getTime() + distance * 24 * 60 * 60 * 1000);
                startInput.value = toYMD(nextDate);
            }
        }

        // 2. Calculate End Date based on Start, Frequency, and Occurrences
        function calculateEndDate() {
            if (!startInput.value) return;
            const start = new Date(startInput.value + 'T12:00:00'); // use noon to avoid timezone issues
            const freq = freqSelect.value;
            const occ = parseInt(occurrencesInput.value) || 1;

            const end = new Date(start);
            if (freq === 'weekly') {
                end.setDate(end.getDate() + (occ - 1) * 7);
            } else if (freq === 'biweekly') {
                end.setDate(end.getDate() + (occ - 1) * 14);
            } else if (freq === 'monthly') {
                end.setMonth(end.getMonth() + (occ - 1));
            }

            endInput.value = toYMD(end);

            // Update preview
            previewCount.textContent = occ;
            previewStart.textContent = toDisplay(start);
            previewEnd.textContent = toDisplay(end);
        }

        // 3. Attach listeners to recalculate everything on change
        typeSelect.addEventListener('change', () => {
            calculateStartDate();
            calculateEndDate();
        });

        startInput.addEventListener('change', calculateEndDate);
        freqSelect.addEventListener('change', calculateEndDate);
        occurrencesInput.addEventListener('input', calculateEndDate);

        // Run on initial load
        calculateStartDate();
        calculateEndDate();
    });
</script>
{{end}}
HTMLEOF
echo "  ✓ plan_bulk.html updated with smart calculation"

echo "▸ [2/2] Build & Restart"
if ! go build ./...; then
    echo "✗ build failed — paste errors"
    exit 1
fi
echo "✓ build OK"

docker compose -f deploy/docker-compose.yml up -d --build
