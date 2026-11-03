// ---- Scheduling: time duration preview ----
(function () {
    const starts = document.getElementById('time-starts');
    const duration = document.getElementById('time-duration');
    const preview = document.getElementById('time-preview');
    if (!starts || !duration || !preview) return;

    function updatePreview() {
        if (!starts.value) { preview.textContent = ''; return; }
        const start = new Date(starts.value);
        const mins = parseInt(duration.value, 10) || 0;
        const end = new Date(start.getTime() + mins * 60000);
        const fmt = (d) => d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        preview.textContent = `${fmt(start)} → ${fmt(end)}  (${mins} min)`;
    }
    starts.addEventListener('input', updatePreview);
    duration.addEventListener('change', updatePreview);
    updatePreview();
})();


// ---- Scheduling: Modal Song Picker (Batch) ----
let stagedSongs = [];

let existingArrangements = [];

function openSongModal(timeId) {
    const modal = document.getElementById('song-modal');
    const timeInput = document.getElementById('modal-plan-time-id');
    if (!modal || !timeInput) return;

    timeInput.value = timeId;

    // Read existing songs for this specific time block
    const timeBlock = document.querySelector(`.time-block[data-time-id="${timeId}"]`);
    const existingStr = timeBlock ? timeBlock.getAttribute('data-existing') : '';
    existingArrangements = existingStr ? existingStr.split(',').filter(Boolean) : [];

    document.getElementById('modal-search').value = '';
    document.getElementById('modal-results').innerHTML = '<p class="muted" style="font-size:0.9em; text-align:center; padding: 2rem 0;">Type 2+ characters to search.</p>';

    stagedSongs = [];
    renderStaged();

    modal.style.display = 'flex';
    document.getElementById('modal-search').focus();
}

function closeSongModal(e) {
    if (e && e.target !== e.currentTarget) return;
    const modal = document.getElementById('song-modal');
    if (modal) modal.style.display = 'none';
}

// Search Logic
(function () {
    const search = document.getElementById('modal-search');
    const results = document.getElementById('modal-results');
    if (!search) return;

    let timer = null;

    async function runSearch() {
        const q = search.value.trim();
        if (q.length < 2) return;

        results.innerHTML = '<p class="muted" style="font-size:0.9em; text-align:center; padding: 1rem;">Searching...</p>';

        try {
            const resp = await fetch('/songs/search?q=' + encodeURIComponent(q), {
                headers: { 'Accept': 'application/json' },
                credentials: 'same-origin',
            });
            if (!resp.ok) return;
            const data = await resp.json();

            results.innerHTML = '';
            if (!Array.isArray(data) || data.length === 0) {
                results.innerHTML = '<p class="muted" style="font-size:0.9em; text-align:center; padding: 1rem;">No matches found.</p>';
                return;
            }

            for (const r of data) {
                const isExisting = existingArrangements.includes(r.arrangement_id);
                const isStaged = stagedSongs.some(s => s.arrangement_id === r.arrangement_id);

                const item = document.createElement('div');
                item.className = 'search-result-item';

                let btnHtml = `<button type="button" class="button-secondary btn-sm">+ Add</button>`;
                if (isExisting) {
                    btnHtml = `<button type="button" class="button-secondary btn-sm" disabled style="opacity:0.5; cursor:not-allowed;">Already Added</button>`;
                } else if (isStaged) {
                    btnHtml = `<button type="button" class="button-secondary btn-sm" disabled style="opacity:0.5; cursor:not-allowed;">Staged</button>`;
                }

                item.innerHTML = `
                    <div>
                        <strong>${r.song_title}</strong>
                        <div class="muted" style="font-size:0.85em;">${r.arrangement_name} · ${r.original_key}</div>
                    </div>
                    ${btnHtml}
                `;

                if (!isExisting && !isStaged) {
                    item.querySelector('button').addEventListener('click', () => stageSong(r));
                }
                results.appendChild(item);
            }
        } catch (e) {
            results.innerHTML = '<p class="muted" style="color:var(--danger); text-align:center; padding: 1rem;">Search failed.</p>';
        }
    }

    search.addEventListener('input', () => {
        clearTimeout(timer);
        timer = setTimeout(runSearch, 250);
    });
})();

// Staging Logic
function stageSong(song) {
    if (existingArrangements.includes(song.arrangement_id)) {
        alert("This arrangement is already in this time block.");
        return;
    }
    if (stagedSongs.some(s => s.arrangement_id === song.arrangement_id)) {
        alert("You have already staged this arrangement.");
        return;
    }

    stagedSongs.push({
        arrangement_id: song.arrangement_id,
        song_title: song.song_title,
        arrangement_name: song.arrangement_name,
        key: song.original_key || '',
        duration: ''
    });
    renderStaged();
}

function renderStaged() {
    const list = document.getElementById('staged-list');
    const btn = document.getElementById('btn-add-batch');
    const count = document.getElementById('staged-count');
    if (!list) return;

    list.innerHTML = '';

    if (stagedSongs.length === 0) {
        list.innerHTML = '<li class="staged-empty">No songs selected yet.</li>';
        btn.disabled = true;
        btn.textContent = 'Add 0 Songs to Plan';
        if(count) count.textContent = '0';
        return;
    }

    stagedSongs.forEach((s, i) => {
        const li = document.createElement('li');
        li.className = 'staged-item';
        li.innerHTML = `
            <input type="hidden" name="arrangement_id" value="${s.arrangement_id}">
            <div class="staged-info">
                <strong>${s.song_title}</strong>
                <span class="muted" style="font-size:0.85em; display:block;">${s.arrangement_name}</span>
            </div>
            <div class="staged-inputs">
                <input type="text" name="key" value="${s.key}" placeholder="Key" title="Key">
                <input type="number" name="duration" placeholder="Min" title="Length in minutes">
                <button type="button" class="link-btn" onclick="removeStaged(${i})" style="color: var(--danger); font-size: 1.2rem; margin-left:0.5rem;">×</button>
            </div>
        `;
        list.appendChild(li);
    });

    btn.disabled = false;
    btn.textContent = `Add ${stagedSongs.length} Song(s) to Plan`;
    if(count) count.textContent = stagedSongs.length;
}

window.removeStaged = function(index) {
    stagedSongs.splice(index, 1);
    renderStaged();
}


// ---- Scheduling: Drag & Drop Reordering ----
(function () {
    let draggedItem = null;

    document.querySelectorAll('.item-row').forEach(function (row) {
        if (!row.dataset.itemId) return;
        row.setAttribute('draggable', 'true');

        row.addEventListener('dragstart', function (e) {
            draggedItem = row;
            row.classList.add('dragging');
            e.dataTransfer.effectAllowed = 'move';
            try { e.dataTransfer.setData('text/plain', row.dataset.itemId); } catch (_) {}
        });

        row.addEventListener('dragend', function () {
            draggedItem = null;
            row.classList.remove('dragging');
            document.querySelectorAll('.item-row').forEach(r => {
                r.classList.remove('drag-over-top', 'drag-over-bottom');
            });
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

            // Clean up classes
            rows.forEach(r => r.classList.remove('drag-over-top', 'drag-over-bottom'));

            // POST the new order for this list
            const timeId = list.closest('.time-block').dataset.timeId;
            const ids = Array.from(list.querySelectorAll('.item-row'))
                .map(r => r.dataset.itemId)
                .filter(Boolean);

            const body = new URLSearchParams();
            body.append('plan_time_id', timeId);
            ids.forEach(id => body.append('item_ids', id));

            // Extract Plan ID from URL (e.g., /plans/123)
            const pathParts = window.location.pathname.split('/');
            const planId = pathParts[2]; // Assuming path is /plans/{id}

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
})();


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
