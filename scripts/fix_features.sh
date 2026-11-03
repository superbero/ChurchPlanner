#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  Polishing the PDF Modal UI"
echo "════════════════════════════════════════════════════════════════"

echo "▸ [1/2] Updating list.html (PDF Modal layout)"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/songs/templates/list.html")
src = p.read_text()

old_modal = '''<!-- PDF Modal -->
<div id="pdf-modal" class="modal-overlay" style="display: none;" onclick="closePdfModal(event)">
    <div class="modal-content" style="max-width: 400px;" onclick="event.stopPropagation()">
        <div class="modal-close" onclick="closePdfModal()">×</div>
        <h2 style="margin-top: 0;">Download PDF</h2>
        <p id="pdf-modal-title" class="muted" style="margin-bottom: 1.5rem;"></p>

        <form id="pdf-form" method="GET" action="">
            <label>Key
                <select name="key">
                    <option value="">Original Key (Default)</option>
                    <option value="C">C</option>
                    <option value="Db">Db (C#)</option>
                    <option value="D">D</option>
                    <option value="Eb">Eb (D#)</option>
                    <option value="E">E</option>
                    <option value="F">F</option>
                    <option value="F#">F# (Gb)</option>
                    <option value="G">G</option>
                    <option value="Ab">Ab (G#)</option>
                    <option value="A">A</option>
                    <option value="Bb">Bb (A#)</option>
                    <option value="B">B</option>
                </select>
            </label>

            <label>Format
                <select name="format">
                    <option value="chords">Chords & Lyrics</option>
                    <option value="numbers">Nashville Numbers</option>
                    <option value="lyrics">Lyrics Only</option>
                </select>
            </label>

            <button type="submit" class="button" style="width: 100%; margin-top: 1rem;">Download PDF</button>
        </form>
    </div>
</div>'''

new_modal = '''<!-- PDF Modal -->
<div id="pdf-modal" class="modal-overlay" style="display: none;" onclick="closePdfModal(event)">
    <div class="modal-content" style="max-width: 450px;" onclick="event.stopPropagation()">
        <div class="modal-close" onclick="closePdfModal()">×</div>
        <div class="modal-body">
            <h2 style="margin-top: 0;">Download PDF</h2>
            <p id="pdf-modal-title" class="muted" style="margin-bottom: 1.5rem; font-size: 1.1em;"></p>

            <form id="pdf-form" method="GET" action="" class="pdf-form">
                <div class="form-group">
                    <label>Key</label>
                    <select name="key">
                        <option value="">Original Key (Default)</option>
                        <option value="C">C</option>
                        <option value="Db">Db (C#)</option>
                        <option value="D">D</option>
                        <option value="Eb">Eb (D#)</option>
                        <option value="E">E</option>
                        <option value="F">F</option>
                        <option value="F#">F# (Gb)</option>
                        <option value="G">G</option>
                        <option value="Ab">Ab (G#)</option>
                        <option value="A">A</option>
                        <option value="Bb">Bb (A#)</option>
                        <option value="B">B</option>
                    </select>
                </div>

                <div class="form-group">
                    <label>Format</label>
                    <select name="format">
                        <option value="chords">Chords & Lyrics</option>
                        <option value="numbers">Nashville Numbers</option>
                        <option value="lyrics">Lyrics Only</option>
                    </select>
                </div>

                <button type="submit" class="button" style="width: 100%; margin-top: 1rem; padding: 0.75rem;">Download PDF</button>
            </form>
        </div>
    </div>
</div>'''

if old_modal in src:
    p.write_text(src.replace(old_modal, new_modal, 1))
    print("  ✓ Modal HTML rewritten with proper classes")
else:
    print("  ! Could not find exact old modal block. Trying regex...")
    import re
    src = re.sub(
        r'<!-- PDF Modal -->.*?<!-- PDF Modal -->',
        new_modal,
        src,
        flags=re.DOTALL
    )
    p.write_text(src)
PYEOF

echo "▸ [2/2] Adding specific CSS for the PDF Modal"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/web/static/css/app.css")
src = p.read_text()

if '.pdf-form' not in src:
    src += '''

/* ── PDF Modal Form ───────────────────────────────────────────── */
.modal-body {
    padding: 2rem;
}
.pdf-form {
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
}
.pdf-form .form-group {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
}
.pdf-form label {
    font-size: 0.9em;
    font-weight: 600;
    color: #475467;
    margin: 0;
}
.pdf-form select {
    width: 100%;
    padding: 0.6rem;
    border: 1px solid var(--border);
    border-radius: 6px;
    font-size: 1rem;
    font-family: inherit;
    background-color: #fff;
    cursor: pointer;
}
.pdf-form select:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px rgba(47, 111, 235, 0.1);
}
'''
    p.write_text(src)
    print("  ✓ CSS added for .pdf-form")
else:
    print("  · CSS already present")
PYEOF

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
