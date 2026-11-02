#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }
if ls migrations/00013_* >/dev/null 2>&1; then
  echo "✗ migrations/00013_* already exists"; exit 1
fi

echo "════════════════════════════════════════════════════════════════"
echo "  1/7  Migration 00013: plan_times.position"
echo "════════════════════════════════════════════════════════════════"
cat > migrations/00013_plan_times_position.sql <<'SQLEOF'
-- +goose Up
ALTER TABLE plan_times ADD COLUMN position INTEGER NOT NULL DEFAULT 0;

-- Backfill position based on current start time so nothing moves initially.
WITH ordered AS (
  SELECT id, ROW_NUMBER() OVER (PARTITION BY plan_id ORDER BY starts_at) - 1 AS rn
  FROM plan_times
)
UPDATE plan_times pt
SET position = o.rn
FROM ordered o
WHERE pt.id = o.id;

-- +goose Down
ALTER TABLE plan_times DROP COLUMN IF EXISTS position;
SQLEOF
echo "  ✓ migration written"

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  2/7  Repository: order times by position + ReorderTimes"
echo "════════════════════════════════════════════════════════════════"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/scheduling/plans.go")
src = p.read_text()

# 1. Update ListTimes to sort by position, then starts_at
src = src.replace(
'''		FROM plan_times WHERE plan_id = $1 ORDER BY starts_at
	`, planID)''',
'''		FROM plan_times WHERE plan_id = $1 ORDER BY position, starts_at
	`, planID)''')

# 2. Add ReorderTimes method
if "func (r *PlanRepository) ReorderTimes" not in src:
    src += '''

// ReorderTimes renumbers the times of a plan in the given order.
func (r *PlanRepository) ReorderTimes(ctx context.Context, planID string, timeIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for i, id := range timeIDs {
		_, err := tx.Exec(ctx, `
			UPDATE plan_times SET position = $2
			WHERE id = $1 AND plan_id = $3
		`, id, i, planID)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
'''
p.write_text(src)
print("  ✓ ListTimes sorted by position, ReorderTimes added")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  3/7  Repository: ReorderItems also updates plan_time_id"
echo "════════════════════════════════════════════════════════════════"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/scheduling/assignments.go")
src = p.read_text()

old = '''// ReorderItems renumbers the items of one time slot in the given order.
// Items not present in itemIDs keep their current positions.
func (r *AssignmentRepository) ReorderItems(ctx context.Context, planID, planTimeID string, itemIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for i, itemID := range itemIDs {
		_, err := tx.Exec(ctx, `
			UPDATE plan_items SET position = $3
			WHERE id = $1 AND plan_id = $2 AND plan_time_id = $4
		`, itemID, planID, i+1, planTimeID)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}'''

new = '''// ReorderItems renumbers the items of one time slot in the given order.
// It also reassigns plan_time_id, so dragging an item from one time slot
// to another works via this same endpoint.
func (r *AssignmentRepository) ReorderItems(ctx context.Context, planID, planTimeID string, itemIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for i, itemID := range itemIDs {
		_, err := tx.Exec(ctx, `
			UPDATE plan_items SET position = $3, plan_time_id = $4
			WHERE id = $1 AND plan_id = $2
		`, itemID, planID, i+1, planTimeID)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}'''

if old not in src:
    print("  ! ReorderItems block not found")
    raise SystemExit(1)
p.write_text(src.replace(old, new, 1))
print("  ✓ ReorderItems updated to support cross-list moves")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  4/7  Handler: ReorderTimes"
echo "════════════════════════════════════════════════════════════════"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/scheduling/handler.go")
src = p.read_text()

if "func (h *Handler) ReorderTimes" not in src:
    src += '''

func (h *Handler) ReorderTimes(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	if err := r.ParseForm(); err != nil {
		httpx.ServerError(w, err)
		return
	}
	ids := r.Form["time_ids"]
	if err := h.plans.ReorderTimes(r.Context(), planID, ids); err != nil {
		httpx.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
'''
p.write_text(src)
print("  ✓ ReorderTimes handler added")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  5/7  Routes: /times/reorder"
echo "════════════════════════════════════════════════════════════════"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/scheduling/routes.go")
src = p.read_text()

if '/times/reorder' in src:
    print("  (already present)")
    raise SystemExit(0)

src = src.replace(
'''		r.Post("/{id}/times", m.handler.AddTime)''',
'''		r.Post("/{id}/times", m.handler.AddTime)
		r.Post("/{id}/times/reorder", m.handler.ReorderTimes)''')

p.write_text(src)
print("  ✓ route added")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  6/7  Template: time drag handle + presentation/media items"
echo "════════════════════════════════════════════════════════════════"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/scheduling/templates/plan_view.html")
src = p.read_text()

# 1. Add drag handle to time header
old_header = '''        <header class="time-header">
            <h3>
                <span class="time-kind">{{.Time.Kind}}</span>
                {{.Time.Name}}
                <span class="muted">· {{.Time.StartsAt.Format "15:04" }} → {{.Time.EndsAt.Format "15:04"}}</span>
            </h3>'''
new_header = '''        <header class="time-header">
            <div class="time-title-group">
                <span class="drag-handle time-drag-handle" title="Drag to reorder times">⋮⋮</span>
                <h3>
                    <span class="time-kind">{{.Time.Kind}}</span>
                    {{.Time.Name}}
                    <span class="muted">· {{.Time.StartsAt.Format "15:04" }} → {{.Time.EndsAt.Format "15:04"}}</span>
                </h3>
            </div>'''
if old_header in src:
    src = src.replace(old_header, new_header, 1)

# 2. Add Presentation/Media to header/note dropdown
old_kinds = '''                            <select name="kind">
                                <option value="header">Header</option>
                                <option value="note">Note</option>
                            </select>'''
new_kinds = '''                            <select name="kind">
                                <option value="header">Header</option>
                                <option value="note">Note</option>
                                <option value="presentation">Presentation (PPT/ProPresenter)</option>
                                <option value="media">Media / Video</option>
                            </select>'''
if old_kinds in src:
    src = src.replace(old_kinds, new_kinds, 1)

# 3. Ensure time-section has a data-time-id and is draggable container
# (we will handle draggable in JS, but adding attribute helps)
src = src.replace(
    '<section class="time-section" data-time-id="{{.Time.ID}}">',
    '<section class="time-section" data-time-id="{{.Time.ID}}">'
)

p.write_text(src)
print("  ✓ template updated (drag handle + media kinds)")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  7/7  JS + CSS: full drag & drop implementation"
echo "════════════════════════════════════════════════════════════════"
# --- JS rewrite for item drag and new time drag ---
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/web/static/js/app.js")
src = p.read_text()

# Replace the old drag-and-drop block
import re
start_marker = "// ---- Scheduling: drag reorder within a time slot ----"
if start_marker in src:
    # Find the end of this block (it's the last block in the file typically)
    end_idx = src.find(start_marker)
    src = src[:end_idx] # truncate everything from here

# Add the new comprehensive drag-and-drop block
src += '''

// ---- Scheduling: drag & drop times and items ----

// Global state for dragging
let draggedItem = null;
let draggedTime = null;

// --- TIMES DRAG ---
document.querySelectorAll('.time-section').forEach(function (section) {
    const handle = section.querySelector('.time-drag-handle');
    if (!handle) return;

    handle.addEventListener('mousedown', function () {
        section.setAttribute('draggable', 'true');
    });
    handle.addEventListener('mouseup', function () {
        section.setAttribute('draggable', 'false');
    });

    section.addEventListener('dragstart', function (e) {
        // Ensure we only drag when the handle was used (or if draggable got set)
        if (section.getAttribute('draggable') !== 'true') {
            e.preventDefault();
            return;
        }
        draggedTime = section;
        section.classList.add('dragging');
        e.dataTransfer.effectAllowed = 'move';
        try { e.dataTransfer.setData('text/plain', 'time'); } catch (_) {}
    });

    section.addEventListener('dragend', function () {
        draggedTime = null;
        section.classList.remove('dragging');
        section.setAttribute('draggable', 'false');
        document.querySelectorAll('.time-section').forEach(s => {
            s.classList.remove('drag-over-top', 'drag-over-bottom');
        });
    });

    section.addEventListener('dragover', function (e) {
        if (!draggedTime || draggedTime === section) return;
        e.preventDefault();
        const rect = section.getBoundingClientRect();
        const isTop = e.clientY < rect.top + rect.height / 2;
        section.classList.toggle('drag-over-top', isTop);
        section.classList.toggle('drag-over-bottom', !isTop);
    });

    section.addEventListener('dragleave', function () {
        section.classList.remove('drag-over-top', 'drag-over-bottom');
    });

    section.addEventListener('drop', async function (e) {
        e.preventDefault();
        section.classList.remove('drag-over-top', 'drag-over-bottom');
        if (!draggedTime || draggedTime === section) return;

        const container = section.parentNode;
        const rect = section.getBoundingClientRect();
        const isTop = e.clientY < rect.top + rect.height / 2;

        if (isTop) container.insertBefore(draggedTime, section);
        else container.insertBefore(draggedTime, section.nextSibling);

        const ids = Array.from(container.querySelectorAll('.time-section'))
            .map(s => s.dataset.timeId)
            .filter(Boolean);

        const body = new URLSearchParams();
        ids.forEach(id => body.append('time_ids', id));

        const planId = location.pathname.split('/').pop();
        try {
            const resp = await fetch(`/plans/${planId}/times/reorder`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                credentials: 'same-origin',
                body: body.toString(),
            });
            if (!resp.ok) console.warn('time reorder failed', resp.status);
        } catch (err) { console.error('time reorder failed', err); }
    });
});


// --- ITEMS DRAG (across lists) ---
document.querySelectorAll('.item-row').forEach(function (row) {
    if (!row.dataset.itemId) return;
    row.setAttribute('draggable', 'true');

    row.addEventListener('dragstart', function (e) {
        draggedItem = row;
        row.classList.add('dragging');
        e.dataTransfer.effectAllowed = 'move';
        try { e.dataTransfer.setData('text/plain', 'item'); } catch (_) {}
        e.stopPropagation(); // prevent section from also dragging
    });

    row.addEventListener('dragend', function () {
        draggedItem = null;
        row.classList.remove('dragging');
        document.querySelectorAll('.item-row').forEach(r => {
            r.classList.remove('drag-over-top', 'drag-over-bottom');
        });
    });
});

document.querySelectorAll('.item-list').forEach(function (list) {
    // Adding drop target to the list itself so we can drop into empty lists
    list.addEventListener('dragover', function (e) {
        if (!draggedItem) return;
        e.preventDefault(); // allow drop
        e.stopPropagation();
    });

    list.addEventListener('drop', async function (e) {
        if (!draggedItem) return;
        e.preventDefault();
        e.stopPropagation();

        // Determine insertion point based on mouse Y
        const rows = Array.from(list.querySelectorAll('.item-row')).filter(r => r.dataset.itemId);
        let inserted = false;
        for (const row of rows) {
            if (row === draggedItem) continue;
            const box = row.getBoundingClientRect();
            if (e.clientY < box.top + box.height / 2) {
                list.insertBefore(draggedItem, row);
                inserted = true;
                break;
            }
        }
        if (!inserted) {
            list.appendChild(draggedItem);
        }

        // POST the new order for this list
        const timeId = list.closest('.time-section').dataset.timeId;
        const ids = Array.from(list.querySelectorAll('.item-row'))
            .map(r => r.dataset.itemId)
            .filter(Boolean);

        const body = new URLSearchParams();
        body.append('plan_time_id', timeId);
        ids.forEach(id => body.append('item_ids', id));

        const planId = location.pathname.split('/').pop();
        try {
            const resp = await fetch(`/plans/${planId}/items/reorder`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                credentials: 'same-origin',
                body: body.toString(),
            });
            if (!resp.ok) console.warn('item reorder failed', resp.status);
            else {
                list.querySelectorAll('.item-row').forEach((r, idx) => {
                    const posEl = r.querySelector('.item-position');
                    if (posEl) posEl.textContent = idx + 1;
                });
            }
        } catch (err) { console.error('item reorder failed', err); }
    });
});

// Also allow dropping onto individual rows for precise placement
document.querySelectorAll('.item-row').forEach(function (row) {
    if (!row.dataset.itemId) return;
    row.addEventListener('dragover', function (e) {
        if (!draggedItem || draggedItem === row) return;
        e.preventDefault();
        e.stopPropagation();
        const rect = row.getBoundingClientRect();
        const isTop = e.clientY < rect.top + rect.height / 2;
        row.classList.toggle('drag-over-top', isTop);
        row.classList.toggle('drag-over-bottom', !isTop);
    });
    row.addEventListener('dragleave', function () {
        row.classList.remove('drag-over-top', 'drag-over-bottom');
    });
});
'''
p.write_text(src)
print("  ✓ app.js rewritten with time + item drag")
PYEOF

# --- CSS additions ---
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/web/static/css/app.css")
src = p.read_text()

if ".time-title-group" in src:
    print("  (already present)")
    raise SystemExit(0)

src += '''

/* Time drag layout */
.time-title-group { display: flex; align-items: center; gap: .5rem; }
.time-drag-handle { font-size: 1.2em; color: #999; margin-top: -.25rem; }
.time-section.dragging { opacity: .4; }
.time-section.drag-over-top    { border-top: 3px solid var(--accent); }
.time-section.drag-over-bottom { border-bottom: 3px solid var(--accent); }

/* Presentation / media styles */
.item-presentation { color: #8b5cf6; font-weight: 600; }
.item-media { color: #0ea5e9; font-weight: 600; }
'''
p.write_text(src)
print("  ✓ app.css updated")
PYEOF

# Cache bust
python3 - <<'PYEOF'
from pathlib import Path
import re
p = Path("internal/web/templates/layout.html")
src = p.read_text()
m = re.search(r'app\.js\?v=(\d+)', src)
if m:
    new_v = int(m.group(1)) + 1
    src = src.replace(m.group(0), f'app.js?v={new_v}')
    print(f"  ✓ app.js bumped to v={new_v}")
p.write_text(src)
PYEOF

echo ""
echo "▸ Build"
if ! go build ./...; then
    echo "✗ build failed — paste errors"
    exit 1
fi
echo "✓ build OK"

docker compose -f deploy/docker-compose.yml up -d --build
echo ""
echo "▸ Logs (20s)"
timeout 20 docker compose -f deploy/docker-compose.yml logs -f app 2>&1 || true

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  Verify"
echo "════════════════════════════════════════════════════════════════"
cat <<'CHECKLIST'

  1. HARD-REFRESH (Cmd/Ctrl+Shift+R).

  2. Drag a time:
       - Hover over the top-left of a time header (the ⋮⋮ grip)
       - Drag the whole section up or down
       - Release → position is saved (reload to verify)

  3. Drag items across lists:
       - Drag a song from "Rehearsal" down into "Service"
       - Drop it anywhere in the Service list
       - Reload → the song moved to Service

  4. Add a Presentation / Media item:
       - In any time block, expand "+ Add a header / note instead"
       - The dropdown now offers "Presentation (PPT/ProPresenter)" and "Media / Video"
       - Add one → it renders with a coloured accent

CHECKLIST
