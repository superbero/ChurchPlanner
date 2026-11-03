#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  Adding Advanced Transpose & PDF Options"
echo "════════════════════════════════════════════════════════════════"

echo "▸ [1/6] Updating arrangement_form.html (Transpose UI)"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/songs/templates/arrangement_form.html")
src = p.read_text()

old_card = '''            <div class="settings-card">
                <h3>Transpose Workspace</h3>
                <p class="muted" style="font-size: 0.85em; margin-top: 0; margin-bottom: 1rem;">
                    Rewrite the chords in the source content to a new key.
                </p>
                <div class="grid-2" style="align-items: end;">
                    <label>Target Key
                        <input type="text" id="target-key" placeholder="e.g. A, Bb" style="text-transform: capitalize;">
                    </label>
                    <button type="button" id="btn-transpose" class="button" style="margin-bottom: 1rem;">Transpose</button>
                </div>
            </div>'''

new_card = '''            <div class="settings-card">
                <h3>Transpose Workspace</h3>
                <p class="muted" style="font-size: 0.85em; margin-top: 0; margin-bottom: 1rem;">
                    Rewrite the chords in the source content.
                </p>

                <label>Target Key
                    <select id="target-key">
                        <option value="">— Choose Key —</option>
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

                <div class="grid-2" style="align-items: end; margin-top: 1rem;">
                    <label>Shift by
                        <select id="shift-semitones">
                            <option value="0">None</option>
                            <option value="1">+1 (Up 1)</option>
                            <option value="2">+2 (Up 2)</option>
                            <option value="-1">-1 (Drop 1)</option>
                            <option value="-2">-2 (Drop 2)</option>
                            <option value="-3">-3 (Drop 3)</option>
                        </select>
                    </label>
                    <button type="button" id="btn-transpose" class="button" style="margin-bottom: 1rem;">Apply</button>
                </div>
            </div>'''

if old_card in src:
    p.write_text(src.replace(old_card, new_card, 1))
    print("  ✓ Transpose UI updated with dropdowns")
else:
    print("  · Transpose UI not matched. Attempting fallback...")
    # Fallback if the exact card was different
    import re
    src = re.sub(
        r'<div class="settings-card">\s*<h3>Transpose Workspace</h3>.*?</div>\s*</div>',
        new_card,
        src,
        flags=re.DOTALL
    )
    p.write_text(src)
PYEOF

echo "▸ [2/6] Updating app.js (Logic for Dropdowns)"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/web/static/js/app.js")
src = p.read_text()

# Find the existing transpose block and replace it
import re
start_marker = "// ---- Arrangement Editor: Chord Transposition ----"
end_marker = "})();"
if start_marker in src:
    start_idx = src.find(start_marker)
    end_idx = src.find(end_marker, start_idx) + len(end_marker)

    new_js = '''// ---- Arrangement Editor: Chord Transposition ----
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
})();'''

    src = src[:start_idx] + new_js + src[end_idx:]
    p.write_text(src)
    print("  ✓ app.js transpose logic updated")
else:
    print("  ! Could not find transpose block in app.js")
PYEOF

echo "▸ [3/6] Updating arrangement_view.html (PDF Options UI)"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/songs/templates/arrangement_view.html")
src = p.read_text()

old_header = '''    <div class="header-actions">
        {{if isEditorOrAdmin .CurrentUser.Role}}
        <a class="button-secondary" href="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}/edit">Edit</a>
        <a class="button-secondary" href="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}/history">History</a>
        {{end}}
        <a class="button" href="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}/pdf?key={{.DisplayKey}}&lang={{.ResolvedLang}}">Print / PDF</a>
    </div>'''

new_header = '''    <div class="header-actions">
        {{if isEditorOrAdmin .CurrentUser.Role}}
        <a class="button-secondary" href="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}/edit">Edit</a>
        <a class="button-secondary" href="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}/history">History</a>
        {{end}}

        <form method="GET" action="/songs/{{.Song.ID}}/arrangements/{{.Arrangement.ID}}/pdf" style="display:flex; gap:0.5rem; align-items:center; margin:0;">
            <input type="hidden" name="key" value="{{.DisplayKey}}">
            {{if .ResolvedLang}}<input type="hidden" name="lang" value="{{.ResolvedLang}}">{{end}}
            <select name="format" style="padding: 0.4rem; border-radius: 6px; border: 1px solid var(--border);">
                <option value="chords">Chords & Lyrics</option>
                <option value="numbers">Nashville Numbers</option>
                <option value="lyrics">Lyrics Only</option>
            </select>
            <button type="submit" class="button">Download PDF</button>
        </form>
    </div>'''

if old_header in src:
    p.write_text(src.replace(old_header, new_header, 1))
    print("  ✓ PDF Options UI added to arrangement_view")
else:
    print("  ! Could not find header actions block.")
PYEOF

echo "▸ [4/6] Updating handler.go (Passing PDF format)"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/songs/handler.go")
src = p.read_text()

# In arrangement download handler (there are two: song default and arrangement specific)
old_pdf = '''	displayKey := r.URL.Query().Get("key")
	lang := strings.TrimSpace(r.URL.Query().Get("lang"))

	data, filename, err := h.arr.RenderPDF(r.Context(), &song, arrID, displayKey, "", lang)'''

new_pdf = '''	displayKey := r.URL.Query().Get("key")
	lang := strings.TrimSpace(r.URL.Query().Get("lang"))
	pdfFormat := strings.TrimSpace(r.URL.Query().Get("format"))

	data, filename, err := h.arr.RenderPDF(r.Context(), &song, arrID, displayKey, "", lang, pdfFormat)'''

if old_pdf in src:
    src = src.replace(old_pdf, new_pdf, 1)
    print("  ✓ handler.go updated for PDF format")
else:
    # Fallback for the other handler
    old_pdf2 = '''	displayKey := r.URL.Query().Get("key")
	qrURL := h.publicBaseURL(r) + "/practice/" + token

	data, filename, err := h.arr.RenderPDF(r.Context(), &song, arrID, displayKey, qrURL, "")'''
    new_pdf2 = '''	displayKey := r.URL.Query().Get("key")
	pdfFormat := strings.TrimSpace(r.URL.Query().Get("format"))
	if pdfFormat == "" { pdfFormat = "chords" }
	qrURL := h.publicBaseURL(r) + "/practice/" + token

	data, filename, err := h.arr.RenderPDF(r.Context(), &song, arrID, displayKey, qrURL, "", pdfFormat)'''
    if old_pdf2 in src:
        src = src.replace(old_pdf2, new_pdf2, 1)
        print("  ✓ handler.go (practice route) updated for PDF format")
    else:
        print("  · PDF handler blocks not found or already updated")
PYEOF

echo "▸ [5/6] Updating arrangement_service.go & pdf.go (Nashville Logic)"
python3 - <<'PYEOF'
from pathlib import Path

# 1. Update RenderPDF signature in arrangement_service.go
p = Path("internal/songs/arrangement_service.go")
src = p.read_text()
old_sig = 'func (s *ArrangementService) RenderPDF(ctx context.Context, song *Song, arrangementID, displayKey, qrURL, lang string) ([]byte, string, error) {'
new_sig = 'func (s *ArrangementService) RenderPDF(ctx context.Context, song *Song, arrangementID, displayKey, qrURL, lang, pdfFormat string) ([]byte, string, error) {'
if old_sig in src:
    src = src.replace(old_sig, new_sig, 1)

    # Update the call to RenderSongPDF
    old_call = 'data, err := RenderSongPDF(song.Title, song.Author, dk, a.Name, a.Sequence, chart, qrURL, resolvedLang)'
    new_call = 'data, err := RenderSongPDF(song.Title, song.Author, dk, a.Name, a.Sequence, chart, qrURL, resolvedLang, pdfFormat)'
    if old_call in src:
        src = src.replace(old_call, new_call, 1)
    p.write_text(src)
    print("  ✓ arrangement_service.go updated")

# 2. Update pdf.go to accept format and convert to Nashville
p = Path("internal/songs/pdf.go")
src = p.read_text()

old_pdf_sig = 'func RenderSongPDF(title, author, key, arrangement, sequence string, chart []RenderedSection, qrURL, language string) ([]byte, error) {'
new_pdf_sig = 'func RenderSongPDF(title, author, key, arrangement, sequence string, chart []RenderedSection, qrURL, language, pdfFormat string) ([]byte, error) {'
if old_pdf_sig in src:
    src = src.replace(old_pdf_sig, new_pdf_sig, 1)

    # Inject Nashville conversion logic
    old_render_loop = '''		pdf.SetFont("Courier", "B", 9)
		for _, c := range line.Chords {
			pdf.SetXY(x0+float64(c.Position)*charW, y0)
			pdf.CellFormat(0, chordH, c.Chord, "", 0, "L", false, 0, "")
		}'''
    new_render_loop = '''		pdf.SetFont("Courier", "B", 9)
		for _, c := range line.Chords {
			chordToRender := c.Chord
			if pdfFormat == "numbers" {
				chordToRender = toNashville(c.Chord, key)
			}

			// If format is "lyrics", we skip rendering the chord entirely
			if pdfFormat != "lyrics" {
				pdf.SetXY(x0+float64(c.Position)*charW, y0)
				pdf.CellFormat(0, chordH, chordToRender, "", 0, "L", false, 0, "")
			}
		}'''
    if old_render_loop in src:
        src = src.replace(old_render_loop, new_render_loop, 1)
        p.write_text(src)
        print("  ✓ pdf.go render loop updated for formats")
    else:
        print("  ! Could not find chord render loop in pdf.go")

# 3. Add the Nashville conversion function to pdf.go
if 'func toNashville(' not in src:
    src += '''

// toNashville converts an absolute chord (e.g., Cmaj7) to its Nashville Number equivalent (e.g., 4maj7)
func toNashville(chord, key string) string {
	keyIdx, ok1 := KeyIndex(key)
	if !ok1 {
		return chord
	}
	// Strip minor from key if present (we just need the root)
	keyRoot := key
	if len(key) > 1 && key[1] == 'm' {
		keyRoot = key[:1]
	}

	match := chordRE.FindStringSubmatch(chord)
	if match == nil {
		return chord
	}
	root, suffix := match[1], match[2]

	chordIdx, ok2 := KeyIndex(root)
	if !ok2 {
		return chord
	}

	interval := (chordIdx - keyIdx + 12) % 12

	// Map intervals to Nashville numbers
	// 1=0, 2=2, 3=4, 4=5, 5=7, 6=9, 7=11
	numberMap := map[int]string{
		0: "1", 1: "b2", 2: "2", 3: "b3", 4: "3", 5: "4",
		6: "b5", 7: "5", 8: "b6", 9: "6", 10: "b7", 11: "7",
	}

	num := numberMap[interval]
	if num == "" {
		num = "?"
	}

	return num + suffix
}
'''
    p.write_text(src)
    print("  ✓ pdf.go Nashville logic added")
PYEOF

echo "▸ [6/6] Build & Restart"
if ! go build ./...; then
    echo "✗ build failed — paste errors"
    exit 1
fi
echo "✓ build OK"

docker compose -f deploy/docker-compose.yml up -d --build
