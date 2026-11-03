#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  Implementing the Service Boundary & Draggable Subsections"
echo "════════════════════════════════════════════════════════════════"

echo "▸ [1/3] Updating plan_view.html with Service Start/End & Drag Handles"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/scheduling/templates/plan_view.html")
src = p.read_text()

# 1. Add the Service Start boundary before the times loop
old_section = '''        <div class="section-header">
            <h2>Order of Service</h2>
        </div>

        {{if .Times}}'''

new_section = '''        <div class="section-header">
            <h2>Order of Service</h2>
        </div>

        <!-- Service Start Boundary -->
        <div class="service-boundary">
            <div class="boundary-time">{{.Plan.ServiceAt.Format "3:04 PM"}}</div>
            <div class="boundary-label">Service Start</div>
        </div>

        {{if .Times}}'''

if old_section in src:
    src = src.replace(old_section, new_section, 1)

# 2. Add the Time Block Drag Handle to the header
old_header = '''                <div class="time-block-header">
                    <div class="time-title">
                        <span class="drag-handle">⋮⋮</span>
                        <span class="time-kind">{{.Time.Kind}}</span>'''
new_header = '''                <div class="time-block-header draggable-time" draggable="true" data-time-id="{{.Time.ID}}">
                    <div class="time-title">
                        <span class="drag-handle" title="Drag to reorder time">⋮⋮</span>
                        <span class="time-kind">{{.Time.Kind}}</span>'''

if old_header in src:
    src = src.replace(old_header, new_header, 1)

# 3. Add the Service End boundary after the times loop
old_end = '''        {{else}}
            <div class="empty-state">
                <p>No service times added yet.</p>
            </div>
        {{end}}'''

new_end = '''        {{else}}
            <div class="empty-state">
                <p>No service times added yet.</p>
            </div>
        {{end}}

        <!-- Service End Boundary -->
        <div class="service-boundary">
            <div class="boundary-time">{{.Plan.ServiceEndsAt.Format "3:04 PM"}}</div>
            <div class="boundary-label">Service End</div>
        </div>'''

if old_end in src:
    src = src.replace(old_end, new_end, 1)

p.write_text(src)
print("  ✓ HTML updated with boundaries and time drag handles")
PYEOF

echo "▸ [2/3] Adding CSS for Boundaries and Time Dragging"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/web/static/css/app.css")
src = p.read_text()

if '.service-boundary' not in src:
    src += '''

/* Service Boundaries (Start / End) */
.service-boundary {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 1rem;
    background: #f8fafc;
    border: 2px dashed #cbd5e1;
    border-radius: var(--radius-md);
    margin-bottom: 1rem;
    color: var(--text-muted);
}
.service-boundary:last-child {
    margin-top: 1rem;
    margin-bottom: 0;
}
.boundary-time {
    font-size: 1.25rem;
    font-weight: 700;
    color: var(--text-main);
    font-family: var(--font-mono);
}
.boundary-label {
    font-size: 0.85rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
}

/* Dragging Times */
.draggable-time {
    cursor: grab;
}
.draggable-time:active {
    cursor: grabbing;
}
.time-block.dragging {
    opacity: 0.5;
    border: 2px dashed var(--brand-500);
}
.time-block.drag-over-top {
    border-top: 4px solid var(--brand-500);
}
.time-block.drag-over-bottom {
    border-bottom: 4px solid var(--brand-500);
}
'''
    p.write_text(src)
    print("  ✓ CSS added for boundaries and time dragging")
else:
    print("  · CSS already present")
PYEOF

echo "▸ [3/3] Updating app.js with Time Block Drag & Drop"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/web/static/js/app.js")
src = p.read_text()

if '// ---- Scheduling: Drag & Drop Times ----' not in src:
    src += '''

// ---- Scheduling: Drag & Drop Times (Subsections) ----
(function () {
    let draggedTime = null;
    const container = document.querySelector('.plan-main'); // The container holding all time blocks

    if (!container) return;

    const timeBlocks = document.querySelectorAll('.time-block');

    timeBlocks.forEach(function (block) {
        const header = block.querySelector('.draggable-time');
        if (!header) return;

        // Make the block draggable by initiating on the header
        header.addEventListener('mousedown', function () {
            block.setAttribute('draggable', 'true');
        });
        header.addEventListener('mouseup', function () {
            block.setAttribute('draggable', 'false');
        });

        block.addEventListener('dragstart', function (e) {
            if (block.getAttribute('draggable') !== 'true') {
                e.preventDefault();
                return;
            }
            draggedTime = block;
            block.classList.add('dragging');
            e.dataTransfer.effectAllowed = 'move';
            try { e.dataTransfer.setData('text/plain', block.dataset.timeId); } catch (_) {}
        });

        block.addEventListener('dragend', function () {
            draggedTime = null;
            block.classList.remove('dragging');
            block.setAttribute('draggable', 'false');
            document.querySelectorAll('.time-block').forEach(b => {
                b.classList.remove('drag-over-top', 'drag-over-bottom');
            });
        });

        block.addEventListener('dragover', function (e) {
            if (!draggedTime || draggedTime === block) return;
            e.preventDefault();
            e.stopPropagation();
            const rect = block.getBoundingClientRect();
            const isTop = e.clientY < rect.top + rect.height / 2;
            block.classList.toggle('drag-over-top', isTop);
            block.classList.toggle('drag-over-bottom', !isTop);
        });

        block.addEventListener('dragleave', function () {
            block.classList.remove('drag-over-top', 'drag-over-bottom');
        });

        block.addEventListener('drop', async function (e) {
            if (!draggedTime || draggedTime === block) return;
            e.preventDefault();
            e.stopPropagation();

            const isTop = e.clientY < block.getBoundingClientRect().top + block.offsetHeight / 2;
            if (isTop) {
                container.insertBefore(draggedTime, block);
            } else {
                container.insertBefore(draggedTime, block.nextSibling);
            }

            // Clean up
            document.querySelectorAll('.time-block').forEach(b => {
                b.classList.remove('drag-over-top', 'drag-over-bottom');
            });

            // Save new order to backend
            const timeIds = Array.from(container.querySelectorAll('.time-block'))
                .map(b => b.dataset.timeId)
                .filter(Boolean);

            const body = new URLSearchParams();
            timeIds.forEach(id => body.append('time_ids', id));

            const pathParts = window.location.pathname.split('/');
            const planId = pathParts[2];

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
})();
'''
    p.write_text(src)
    print("  ✓ Time drag & drop logic added to app.js")
else:
    print("  · Time drag & drop logic already present")
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
