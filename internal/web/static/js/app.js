// htmx or progressive enhancement hooks go here.
console.debug("churchplanner ui ready");


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
        preview.textContent = `${fmt(start)} \u2192 ${fmt(end)}  (${mins} min)`;
    }
    starts.addEventListener('input', updatePreview);
    duration.addEventListener('change', updatePreview);
    updatePreview();
})();

// ---- Scheduling: song picker (multi-instance) ----
(function () {
    document.querySelectorAll('.song-picker-form').forEach(function (form) {
        const search = form.querySelector('.song-search');
        const picker = form.querySelector('.song-picker');
        const titleHidden = form.querySelector('.song-title-hidden');
        const keyInput = form.querySelector('.song-key');
        if (!search || !picker) return;

        let timer = null;
        async function runSearch() {
            const q = search.value.trim();
            try {
                const resp = await fetch('/songs/search?q=' + encodeURIComponent(q), {
                    headers: { 'Accept': 'application/json' },
                    credentials: 'same-origin',
                });
                if (!resp.ok) return;
                const results = await resp.json();
                picker.innerHTML = '';
                if (!Array.isArray(results) || results.length === 0) {
                    const o = document.createElement('option');
                    o.value = '';
                    o.textContent = q ? 'No matches' : 'No songs in library yet';
                    picker.appendChild(o);
                    return;
                }
                for (const r of results) {
                    const opt = document.createElement('option');
                    opt.value = r.arrangement_id;
                    opt.dataset.title = r.song_title;
                    opt.dataset.key = r.original_key || '';
                    const keyPart = r.original_key ? ' \u00b7 ' + r.original_key : '';
                    opt.textContent = r.song_title + ' \u2014 ' + r.arrangement_name + keyPart;
                    picker.appendChild(opt);
                }
            } catch (e) { console.error('song search failed', e); }
        }
        search.addEventListener('input', function () {
            clearTimeout(timer);
            timer = setTimeout(runSearch, 200);
        });
        search.addEventListener('focus', function () {
            if (picker.options.length <= 1) runSearch();
        });
        picker.addEventListener('change', function () {
            const opt = picker.options[picker.selectedIndex];
            if (!opt || !opt.value) {
                if (titleHidden) titleHidden.value = '';
                return;
            }
            if (titleHidden) titleHidden.value = opt.dataset.title || '';
            if (keyInput && !keyInput.value) keyInput.value = opt.dataset.key || '';
        });
    });
})();



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


// ---- People: Skill Add/Remove ----
(function () {
    const picker = document.getElementById('skill-picker');
    const addBtn = document.getElementById('add-skill-btn');
    const panel = document.getElementById('active-skills');
    if (!picker || !addBtn || !panel) return;

    addBtn.addEventListener('click', function () {
        const opt = picker.options[picker.selectedIndex];
        if (!opt || !opt.value) return;

        const id = opt.value;
        const team = opt.dataset.team;
        const name = opt.dataset.name;

        // Create chip
        const chip = document.createElement('div');
        chip.className = 'skill-chip';
        chip.dataset.id = id;
        chip.innerHTML = `
            <span><strong>${team}</strong> · ${name}</span>
            <button type="button" class="remove-skill" title="Remove">×</button>
            <input type="hidden" name="position_ids" value="${id}">
        `;

        panel.appendChild(chip);
        picker.remove(picker.selectedIndex);
        picker.selectedIndex = 0;
    });

    panel.addEventListener('click', function (e) {
        if (!e.target.classList.contains('remove-skill')) return;
        const chip = e.target.closest('.skill-chip');
        if (!chip) return;

        const id = chip.dataset.id;
        const team = chip.dataset.team;
        const name = chip.dataset.name;

        // Add option back to dropdown
        const newOpt = document.createElement('option');
        newOpt.value = id;
        newOpt.dataset.team = team;
        newOpt.dataset.name = name;
        newOpt.textContent = `${team} · ${name}`;
        picker.appendChild(newOpt);

        // Sort options alphabetically (optional, but nice)
        const opts = Array.from(picker.options).slice(1); // skip placeholder
        opts.sort((a, b) => a.textContent.localeCompare(b.textContent));

        // Rebuild dropdown
        picker.innerHTML = '<option value="">— Select a skill to add —</option>';
        opts.forEach(o => picker.appendChild(o));

        chip.remove();
    });
})();

// ---- Arrangement Editor: Chord Transposition ----
(function () {
    const btn = document.getElementById('btn-transpose');
    if (!btn) return;

    const targetKeySelect = document.getElementById('target-key');
    const shiftSelect = document.getElementById('shift-semitones');
    const sourceTextarea = document.getElementById('source_content');
    const originalKeyInput = document.querySelector('input[name="original_key"]');

    const notesSharp = ['C', 'C#', 'D', 'D#', 'E', 'F', 'F#', 'G', 'G#', 'A', 'A#', 'B'];
    const notesFlat  = ['C', 'Db', 'D', 'Eb', 'E', 'F', 'Gb', 'G', 'Ab', 'A', 'Bb', 'B'];
    const flatKeys = ['F', 'Bb', 'Eb', 'Ab', 'Db', 'Gb', 'Cb'];

    function getKeyIndex(key) {
        let k = key.trim();
        if (!k) return -1;
        let root = k.charAt(0).toUpperCase() + k.slice(1).toLowerCase();
        if (root.length > 1 && root[1] === 'B') root = root[0] + 'b';
        if (notesSharp.includes(root)) return notesSharp.indexOf(root);
        if (notesFlat.includes(root)) return notesFlat.indexOf(root);
        return -1;
    }

    function transposeChord(chord, semitones, useFlats) {
        const match = chord.match(/^([A-G][#b]?)(.*)$/);
        if (!match) return chord;

        let root = match[1];
        let suffix = match[2];

        let idx = notesSharp.indexOf(root);
        if (idx === -1) idx = notesFlat.indexOf(root);
        if (idx === -1) return chord;

        let newIdx = (idx + semitones) % 12;
        if (newIdx < 0) newIdx += 12;

        const newRoot = useFlats ? notesFlat[newIdx] : notesSharp[newIdx];
        return newRoot + suffix;
    }

    btn.addEventListener('click', function () {
        const targetKey = targetKeySelect.value;
        const shift = parseInt(shiftSelect.value, 10);
        const currentKey = originalKeyInput ? originalKeyInput.value.trim() : '';

        if (!currentKey) {
            alert('Original key is empty. Please set it in Basic Settings first.');
            return;
        }

        let semitones = 0;

        // If a target key is selected, calculate the shift from current key to target
        if (targetKey) {
            const oldIdx = getKeyIndex(currentKey);
            const newIdx = getKeyIndex(targetKey);
            if (oldIdx === -1 || newIdx === -1) {
                alert('Invalid key format.');
                return;
            }
            semitones = newIdx - oldIdx;
        } else if (shift !== 0) {
            semitones = shift;
        } else {
            alert('Please select a Target Key or a Shift amount.');
            return;
        }

        // Simplify to shortest path
        if (semitones > 6) semitones -= 12;
        if (semitones < -6) semitones += 12;

        if (semitones === 0) {
            alert('No transposition needed.');
            return;
        }

        // Determine if new key uses flats
        let newKeyIdx = (getKeyIndex(currentKey) + semitones) % 12;
        if (newKeyIdx < 0) newKeyIdx += 12;
        const newKeyRoot = notesFlat[newKeyIdx]; // Default to flat spelling for new key
        const useFlats = flatKeys.includes(newKeyRoot) || notesFlat.includes(newKeyRoot);

        let text = sourceTextarea.value;
        text = text.replace(/\[([^\]]+)\]/g, function (match, chord) {
            const parts = chord.split('/');
            const transposedRoot = transposeChord(parts[0], semitones, useFlats);

            if (parts.length > 1) {
                const transposedBass = transposeChord(parts[1], semitones, useFlats);
                return `[${transposedRoot}/${transposedBass}]`;
            }
            return `[${transposedRoot}]`;
        });

        sourceTextarea.value = text;

        if (originalKeyInput) {
            // Update original key to the new key
            originalKeyInput.value = targetKey || (useFlats ? notesFlat[newKeyIdx] : notesSharp[newKeyIdx]);
        }

        targetKeySelect.value = '';
        shiftSelect.value = '0';
    });
})();
