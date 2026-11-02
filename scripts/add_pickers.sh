#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

OK=0; FAIL=0
ok()   { echo "  ✓ $1"; OK=$((OK+1)); }
fail() { echo "  ✗ $1"; FAIL=$((FAIL+1)); }
skip() { echo "  · $1 (already present)"; OK=$((OK+1)); }

echo "════════════════════════════════════════════════════════════════"
echo "  Diagnostics"
echo "════════════════════════════════════════════════════════════════"

echo "▸ Migration file:"
ls -la migrations/00013_* 2>/dev/null || echo "  (missing!)"

echo ""
echo "▸ Is 00013 applied in the DB?"
if docker compose -f deploy/docker-compose.yml ps db 2>/dev/null | grep -q "Up"; then
  APPLIED=$(docker compose -f deploy/docker-compose.yml exec -T db \
    psql -U church -d churchplanner -tA -c \
    "SELECT is_applied FROM goose_db_version WHERE version_id = 13 ORDER BY id DESC LIMIT 1;" 2>/dev/null || echo "?")
  echo "  goose_db_version version 13 is_applied = ${APPLIED:-not found}"
else
  echo "  (db not running)"
fi

echo ""
echo "▸ Check which code patches are already in place:"
grep -q 'ORDER BY position, starts_at' internal/scheduling/plans.go && echo "  ✓ ListTimes ORDER BY position" || echo "  ✗ ListTimes ORDER BY position"
grep -q 'func (r \*PlanRepository) ReorderTimes' internal/scheduling/plans.go && echo "  ✓ ReorderTimes in plans.go" || echo "  ✗ ReorderTimes in plans.go"
grep -q 'plan_time_id = \$4' internal/scheduling/assignments.go && echo "  ✓ ReorderItems sets plan_time_id" || echo "  ✗ ReorderItems sets plan_time_id"
grep -q 'func (h \*Handler) ReorderTimes' internal/scheduling/handler.go && echo "  ✓ ReorderTimes handler" || echo "  ✗ ReorderTimes handler"
grep -q '/times/reorder' internal/scheduling/routes.go && echo "  ✓ /times/reorder route" || echo "  ✗ /times/reorder route"
grep -q 'time-drag-handle' internal/scheduling/templates/plan_view.html && echo "  ✓ time drag handle in template" || echo "  ✗ time drag handle in template"
grep -q 'value="presentation"' internal/scheduling/templates/plan_view.html && echo "  ✓ presentation/media kinds" || echo "  ✗ presentation/media kinds"
grep -q 'TIMES DRAG' internal/web/static/js/app.js && echo "  ✓ JS time drag block" || echo "  ✗ JS time drag block"
grep -q '.time-title-group' internal/web/static/css/app.css && echo "  ✓ CSS time drag" || echo "  ✗ CSS time drag"

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  Applying missing patches (idempotent)"
echo "════════════════════════════════════════════════════════════════"

# ────── 1. plans.go: ListTimes ORDER BY + ReorderTimes ──────
echo ""
echo "▸ 1/8  plans.go"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/scheduling/plans.go")
src = p.read_text()
changed = False

if 'ORDER BY position, starts_at' not in src:
    old = 'FROM plan_times WHERE plan_id = $1 ORDER BY starts_at'
    new = 'FROM plan_times WHERE plan_id = $1 ORDER BY position, starts_at'
    if old in src:
        src = src.replace(old, new, 1)
        changed = True
        print("  ✓ ListTimes ORDER BY updated")
    else:
        print("  ! ListTimes ORDER BY pattern not found")

if 'func (r *PlanRepository) ReorderTimes' not in src:
    src += '''

func (r *PlanRepository) ReorderTimes(ctx context.Context, planID string, timeIDs []string) error {
\ttx, err := r.db.Begin(ctx)
\tif err != nil {
\t\treturn err
\t}
\tdefer tx.Rollback(ctx)
\tfor i, id := range timeIDs {
\t\t_, err := tx.Exec(ctx, `
\t\t\tUPDATE plan_times SET position = $2
\t\t\tWHERE id = $1 AND plan_id = $3
\t\t`, id, i, planID)
\t\tif err != nil {
\t\t\treturn err
\t\t}
\t}
\treturn tx.Commit(ctx)
}
'''
    changed = True
    print("  ✓ ReorderTimes added")

if changed:
    p.write_text(src)
PYEOF
[ $? -eq 0 ] && ok "plans.go" || fail "plans.go"

# ────── 2. assignments.go: ReorderItems plan_time_id ──────
echo ""
echo "▸ 2/8  assignments.go"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/scheduling/assignments.go")
src = p.read_text()

old = '''			UPDATE plan_items SET position = $3
			WHERE id = $1 AND plan_id = $2 AND plan_time_id = $4
		`, itemID, planID, i+1, planTimeID)'''
new = '''			UPDATE plan_items SET position = $3, plan_time_id = $4
			WHERE id = $1 AND plan_id = $2
		`, itemID, planID, i+1, planTimeID)'''

if new in src:
    print("  · ReorderItems already sets plan_time_id")
elif old in src:
    p.write_text(src.replace(old, new, 1))
    print("  ✓ ReorderItems updated")
else:
    print("  ! ReorderItems pattern not found")
PYEOF
[ $? -eq 0 ] && ok "assignments.go" || fail "assignments.go"

# ────── 3. handler.go: ReorderTimes ──────
echo ""
echo "▸ 3/8  handler.go"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/scheduling/handler.go")
src = p.read_text()

if 'func (h *Handler) ReorderTimes' in src:
    print("  · ReorderTimes already present")
    raise SystemExit(0)

src += '''

func (h *Handler) ReorderTimes(w http.ResponseWriter, r *http.Request) {
\tplanID := chi.URLParam(r, "id")
\tif err := r.ParseForm(); err != nil {
\t\thttpx.ServerError(w, err)
\t\treturn
\t}
\tids := r.Form["time_ids"]
\tif err := h.plans.ReorderTimes(r.Context(), planID, ids); err != nil {
\t\thttpx.ServerError(w, err)
\t\treturn
\t}
\tw.WriteHeader(http.StatusNoContent)
}
'''
p.write_text(src)
print("  ✓ ReorderTimes handler added")
PYEOF
[ $? -eq 0 ] && ok "handler.go" || fail "handler.go"

# ────── 4. routes.go: /times/reorder ──────
echo ""
echo "▸ 4/8  routes.go"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/scheduling/routes.go")
src = p.read_text()

if '/times/reorder' in src:
    print("  · route already present")
    raise SystemExit(0)

old = '\t\tr.Post("/{id}/times", m.handler.AddTime)'
new = '\t\tr.Post("/{id}/times", m.handler.AddTime)\n\t\tr.Post("/{id}/times/reorder", m.handler.ReorderTimes)'

if old not in src:
    print("  ! times route not found")
    raise SystemExit(1)
p.write_text(src.replace(old, new, 1))
print("  ✓ route added")
PYEOF
[ $? -eq 0 ] && ok "routes.go" || fail "routes.go"

# ────── 5. plan_view.html: drag handle + presentation kinds ──────
echo ""
echo "▸ 5/8  plan_view.html"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/scheduling/templates/plan_view.html")
src = p.read_text()
changed = False

# 5a. Time header drag handle
if 'time-drag-handle' not in src:
    old = '''        <header class="time-header">
            <h3>
                <span class="time-kind">{{.Time.Kind}}</span>
                {{.Time.Name}}
                <span class="muted">· {{.Time.StartsAt.Format "15:04" }} → {{.Time.EndsAt.Format "15:04"}}</span>
            </h3>'''
    new = '''        <header class="time-header">
            <div class="time-title-group">
                <span class="drag-handle time-drag-handle" title="Drag to reorder times">⋮⋮</span>
                <h3>
                    <span class="time-kind">{{.Time.Kind}}</span>
                    {{.Time.Name}}
                    <span class="muted">· {{.Time.StartsAt.Format "15:04" }} → {{.Time.EndsAt.Format "15:04"}}</span>
                </h3>
            </div>'''
    if old in src:
        src = src.replace(old, new, 1)
        changed = True
        print("  ✓ drag handle added")
    else:
        print("  ! time header pattern not found")
else:
    print("  · drag handle already present")

# 5b. Presentation + media kinds
if 'value="presentation"' not in src:
    old = '''                            <select name="kind">
                                <option value="header">Header</option>
                                <option value="note">Note</option>
                            </select>'''
    new = '''                            <select name="kind">
                                <option value="header">Header</option>
                                <option value="note">Note</option>
                                <option value="presentation">Presentation (PPT/ProPresenter)</option>
                                <option value="media">Media / Video</option>
                            </select>'''
    if old in src:
        src = src.replace(old, new, 1)
        changed = True
        print("  ✓ presentation/media kinds added")
    else:
        print("  ! kind select pattern not found")
else:
    print("  · presentation/media already present")

if changed:
    p.write_text(src)
PYEOF
[ $? -eq 0 ] && ok "plan_view.html" || fail "plan_view.html"

# ────── 6. app.js: time drag block ──────
echo ""
echo "▸ 6/8  app.js"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/web/static/js/app.js")
src = p.read_text()

if 'TIMES DRAG' in src:
    print("  · time drag already present")
    raise SystemExit(0)

# If old drag block exists, truncate from there and append new block
start_marker = "// ---- Scheduling: drag reorder within a time slot ----"
if start_marker in src:
    src = src[:src.find(start_marker)]

src += '''

// ---- Scheduling: drag & drop times and items ----
let draggedItem = null;
let draggedTime = null;

document.querySelectorAll('.time-section').forEach(function (section) {
    const handle = section.querySelector('.time-drag-handle');
    if (!handle) return;
    handle.addEventListener('mousedown', function () { section.setAttribute('draggable', 'true'); });
    handle.addEventListener('mouseup', function () { section.setAttribute('draggable', 'false'); });
    section.addEventListener('dragstart', function (e) {
        if (section.getAttribute('draggable') !== 'true') { e.preventDefault(); return; }
        draggedTime = section;
        section.classList.add('dragging');
        e.dataTransfer.effectAllowed = 'move';
        try { e.dataTransfer.setData('text/plain', 'time'); } catch (_) {}
    });
    section.addEventListener('dragend', function () {
        draggedTime = null;
        section.classList.remove('dragging');
        section.setAttribute('draggable', 'false');
        document.querySelectorAll('.time-section').forEach(s => s.classList.remove('drag-over-top', 'drag-over-bottom'));
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
        const ids = Array.from(container.querySelectorAll('.time-section')).map(s => s.dataset.timeId).filter(Boolean);
        const body = new URLSearchParams();
        ids.forEach(id => body.append('time_ids', id));
        const planId = location.pathname.split('/').pop();
        try {
            const resp = await fetch('/plans/' + planId + '/times/reorder', {
                method: 'POST',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                credentials: 'same-origin',
                body: body.toString(),
            });
            if (!resp.ok) console.warn('time reorder failed', resp.status);
        } catch (err) { console.error('time reorder failed', err); }
    });
});

document.querySelectorAll('.item-row').forEach(function (row) {
    if (!row.dataset.itemId) return;
    row.setAttribute('draggable', 'true');
    row.addEventListener('dragstart', function (e) {
        draggedItem = row;
        row.classList.add('dragging');
        e.dataTransfer.effectAllowed = 'move';
        try { e.dataTransfer.setData('text/plain', 'item'); } catch (_) {}
        e.stopPropagation();
    });
    row.addEventListener('dragend', function () {
        draggedItem = null;
        row.classList.remove('dragging');
        document.querySelectorAll('.item-row').forEach(r => r.classList.remove('drag-over-top', 'drag-over-bottom'));
    });
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

document.querySelectorAll('.item-list').forEach(function (list) {
    list.addEventListener('dragover', function (e) {
        if (!draggedItem) return;
        e.preventDefault();
        e.stopPropagation();
    });
    list.addEventListener('drop', async function (e) {
        if (!draggedItem) return;
        e.preventDefault();
        e.stopPropagation();
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
        if (!inserted) list.appendChild(draggedItem);
        const timeId = list.closest('.time-section').dataset.timeId;
        const ids = Array.from(list.querySelectorAll('.item-row')).map(r => r.dataset.itemId).filter(Boolean);
        const body = new URLSearchParams();
        body.append('plan_time_id', timeId);
        ids.forEach(id => body.append('item_ids', id));
        const planId = location.pathname.split('/').pop();
        try {
            const resp = await fetch('/plans/' + planId + '/items/reorder', {
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
'''
p.write_text(src)
print("  ✓ time/item drag JS appended")
PYEOF
[ $? -eq 0 ] && ok "app.js" || fail "app.js"

# ────── 7. CSS ──────
echo ""
echo "▸ 7/8  app.css"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/web/static/css/app.css")
src = p.read_text()

if '.time-title-group' in src:
    print("  · CSS already present")
    raise SystemExit(0)

src += '''

.time-title-group { display: flex; align-items: center; gap: .5rem; }
.time-drag-handle { font-size: 1.2em; color: #999; margin-top: -.25rem; }
.time-section.dragging { opacity: .4; }
.time-section.drag-over-top    { border-top: 3px solid var(--accent); }
.time-section.drag-over-bottom { border-bottom: 3px solid var(--accent); }
.item-presentation { color: #8b5cf6; font-weight: 600; }
.item-media { color: #0ea5e9; font-weight: 600; }
'''
p.write_text(src)
print("  ✓ CSS appended")
PYEOF
[ $? -eq 0 ] && ok "app.css" || fail "app.css"

# ────── 8. Cache bust ──────
echo ""
echo "▸ 8/8  layout.html cache-bust"
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
elif 'app.js?v=' not in src:
    src = src.replace('src="/static/js/app.js"', 'src="/static/js/app.js?v=4"', 1)
    print("  ✓ added ?v=4")
p.write_text(src)
PYEOF
[ $? -eq 0 ] && ok "layout.html" || fail "layout.html"

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  Build + restart"
echo "════════════════════════════════════════════════════════════════"
echo "  Patches OK: $OK   Failed: $FAIL"

if ! go build ./...; then
    echo ""
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

  1. HARD-REFRESH (Cmd/Ctrl+Shift+R) — the app.js version was bumped.

  2. Drag times:
       - Grab the ⋮⋮ grip at the top-left of a time header
       - Drag it above or below another time
       - Reload → order preserved

  3. Drag songs across times:
       - Drag a song from one time section into another
       - Reload → it moved

  4. Add media:
       - Expand "+ Add a header / note instead" inside any time
       - The dropdown now offers "Presentation" and "Media / Video"

CHECKLIST
