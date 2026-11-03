#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  Fixing Song Detail Layout & Missing Buttons"
echo "════════════════════════════════════════════════════════════════"

echo "▸ [1/2] Updating detail.html"
cat > internal/songs/templates/detail.html <<'HTMLEOF'
{{define "content"}}
<div class="page-header song-header">
    <div>
        <h1>{{.Song.Title}} {{if .Song.Archived}}<span class="chip">archived</span>{{end}}</h1>
        <p class="author">{{.Song.Author}}</p>

        <div class="meta-chips">
            {{if .Song.CCLINumber}}<span class="meta-chip" title="CCLI Number"><strong>CCLI:</strong> {{.Song.CCLINumber}}</span>{{end}}
            {{if .Song.CopyrightHolder}}<span class="meta-chip" title="Copyright Holder">© {{.Song.CopyrightHolder}}{{if .Song.CopyrightDate}} {{.Song.CopyrightDate}}{{end}}</span>{{end}}
            {{if .Song.Publisher}}<span class="meta-chip"><strong>Publisher:</strong> {{.Song.Publisher}}</span>{{end}}
            {{if .Song.Scripture}}<span class="meta-chip"><strong>Scripture:</strong> {{.Song.Scripture}}</span>{{end}}
            {{range .Tags}}<span class="chip">{{.Name}}</span>{{end}}
        </div>
    </div>

    <div class="header-actions">
        {{if .Song.YouTubeURL}}
        <a class="button-secondary" href="{{.Song.YouTubeURL}}" target="_blank" rel="noopener">▶ Watch Video</a>
        {{end}}
        <a class="button" href="/songs/{{.Song.ID}}/arrangements/new">Add Arrangement</a>
        {{if isEditorOrAdmin .CurrentUser.Role}}
        <a class="button-secondary" href="/songs/{{.Song.ID}}/edit">Edit Metadata</a>
        {{end}}
    </div>
</div>

<div class="song-layout">

    <!-- LEFT COLUMN: Arrangements -->
    <div class="song-main">
        <h2>Arrangements</h2>
        {{if .Arrangements}}
        <div class="arrangement-list">
            {{range .Arrangements}}
            <div class="arrangement-card {{if .Arrangement.Archived}}archived{{end}}">
                <div class="arrangement-info">
                    <h3>
                        <a href="/songs/{{$.Song.ID}}/arrangements/{{.Arrangement.ID}}">{{.Arrangement.Name}}</a>
                        {{if .Arrangement.IsDefault}}<span class="chip chip-default">Default</span>{{end}}
                        {{if .Arrangement.Archived}}<span class="chip">archived</span>{{end}}
                    </h3>
                    <div class="arrangement-meta">
                        <span><strong>Key:</strong> {{.Arrangement.OriginalKey}}</span>
                        {{if .Arrangement.BPM}}<span><strong>BPM:</strong> {{.Arrangement.BPM}}</span>{{end}}
                        {{if .Arrangement.LengthSeconds}}<span><strong>Length:</strong> {{.Arrangement.LengthSeconds}}s</span>{{end}}
                    </div>
                    {{if .Keys}}
                    <div class="arrangement-keys">
                        {{range .Keys}}<span class="key-badge">{{.Name}}: {{.Key}}</span>{{end}}
                    </div>
                    {{end}}
                </div>

                <div class="arrangement-actions">
                    <a href="/songs/{{$.Song.ID}}/arrangements/{{.Arrangement.ID}}" class="button">View Chart</a>
                    {{if isEditorOrAdmin $.CurrentUser.Role}}
                    <div class="secondary-actions">
                        <a href="/songs/{{$.Song.ID}}/arrangements/{{.Arrangement.ID}}/edit">Edit</a> ·
                        <a href="/songs/{{$.Song.ID}}/arrangements/{{.Arrangement.ID}}/pdf">PDF</a>
                        {{if not .Arrangement.IsDefault}}
                            · <form method="post" action="/songs/{{$.Song.ID}}/arrangements/{{.Arrangement.ID}}/set-default" style="display:inline">
                                <button type="submit" class="link-btn" style="color:var(--accent)">Make Default</button>
                            </form>
                        {{end}}
                    </div>
                    {{else}}
                    <div class="secondary-actions">
                        <a href="/songs/{{$.Song.ID}}/arrangements/{{.Arrangement.ID}}/pdf">PDF</a>
                    </div>
                    {{end}}
                </div>
            </div>
            {{end}}
        </div>
        {{else}}
        <div class="empty-state">
            <p>No arrangements yet. You need an arrangement to start building a chord chart.</p>
            <a href="/songs/{{.Song.ID}}/arrangements/new" class="button">Create First Arrangement</a>
        </div>
        {{end}}

        {{if .Song.Notes}}
        <h2 style="margin-top: 2rem;">Notes</h2>
        <div class="song-notes">{{.Song.Notes}}</div>
        {{end}}
    </div>

    <!-- RIGHT COLUMN: Sidebar (Share & History) -->
    <div class="song-sidebar">

        <!-- Share for Practice -->
        <div class="sidebar-card">
            <h3>Share for practice</h3>
            <p class="muted" style="font-size: 0.9em; margin-bottom: 1rem;">
                Generate a link volunteers can open without logging in. They'll see the chart, YouTube video, and a PDF download.
            </p>
            <form method="post" action="/songs/{{.Song.ID}}/practice-links" class="practice-form">
                <div class="input-group">
                    <input type="number" name="ttl_days" value="30" min="1" max="365" required style="width: 70px;">
                    <span class="muted">days</span>
                    <button type="submit" class="button" style="margin-left:auto;">Generate Link</button>
                </div>
            </form>
            {{if .PracticeToken}}
            <div class="practice-result">
                <strong>New link:</strong> <a href="/practice/{{.PracticeToken}}" target="_blank">/practice/{{.PracticeToken}}</a>
            </div>
            {{end}}
        </div>

        <!-- Key History -->
        <div class="sidebar-card">
            <h3>Key History</h3>
            <p class="muted" style="font-size: 0.9em; margin-bottom: 1rem;">Record which key was used on a specific date.</p>

            <form method="post" action="/songs/{{.Song.ID}}/history" class="history-form">
                <div class="form-row">
                    <div class="form-group">
                        <label>Date</label>
                        <input type="date" name="used_on" required>
                    </div>
                    <div class="form-group" style="flex: 0 0 80px;">
                        <label>Key</label>
                        <input type="text" name="key" value="{{.Song.Key}}" required placeholder="G">
                    </div>
                </div>
                <div class="form-group">
                    <label>Notes (optional)</label>
                    <input type="text" name="notes" placeholder="e.g. Sunday Service">
                </div>
                <button type="submit" class="button-secondary" style="width: 100%; margin-top: 0.5rem;">Record Key</button>
            </form>

            {{if .History}}
            <table class="history-table">
                <thead><tr><th>Date</th><th>Key</th><th>Notes</th></tr></thead>
                <tbody>
                {{range .History}}
                    <tr>
                        <td>{{.UsedOn.Format "2 Jan 2006"}}</td>
                        <td><strong>{{.Key}}</strong></td>
                        <td class="muted">{{.Notes}}</td>
                    </tr>
                {{end}}
                </tbody>
            </table>
            {{else}}
            <p class="muted" style="font-size: 0.9em; margin-top: 1rem; text-align: center;">No history recorded.</p>
            {{end}}
        </div>

        {{if isEditorOrAdmin .CurrentUser.Role}}
        <form method="post" action="/songs/{{.Song.ID}}/delete" onsubmit="return confirm('Delete song and all arrangements?')">
            <button type="submit" class="link-btn" style="color:#b42318; width: 100%; margin-top: 1rem;">Delete Song</button>
        </form>
        {{end}}
    </div>
</div>
{{end}}
HTMLEOF
echo "  ✓ detail.html rewritten"

echo "▸ [2/2] Fixing CSS for Arrangement Actions and Sidebar Forms"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/web/static/css/app.css")
src = p.read_text()

# Remove old broken CSS
import re
src = re.sub(r'/\* Arrangements List \*/.*?(?=/\* Sidebar Cards \*/)', '', src, flags=re.DOTALL)
src = re.sub(r'/\* Sidebar Cards \*/.*?(?=/\* Practice Form \*/)', '', src, flags=re.DOTALL)
src = re.sub(r'/\* Practice Form \*/.*?(?=/\* History Form \*/)', '', src, flags=re.DOTALL)
src = re.sub(r'/\* History Form \*/.*?(?=/\* History Table \*/)', '', src, flags=re.DOTALL)

if '.arrangement-actions' not in src:
    src += '''

/* ── Arrangement List ─────────────────────────────────────────── */
.arrangement-list { display: flex; flex-direction: column; gap: 1rem; }
.arrangement-card {
    background: #fff;
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 1.25rem;
    display: flex;
    justify-content: space-between;
    align-items: center;
    transition: border-color 0.15s, box-shadow 0.15s;
}
.arrangement-card:hover { border-color: var(--accent); box-shadow: 0 4px 6px -1px rgba(0,0,0,0.05); }
.arrangement-card.archived { opacity: 0.6; }

.arrangement-info h3 { margin: 0 0 0.5rem 0; font-size: 1.2em; }
.arrangement-info h3 a { color: var(--accent); text-decoration: none; }
.arrangement-info h3 a:hover { text-decoration: underline; }
.chip-default { background: #d1fadf; color: #05603a; }

.arrangement-meta { display: flex; gap: 1rem; font-size: 0.9em; color: #475467; margin-bottom: 0.5rem; }
.arrangement-keys { display: flex; gap: 0.5rem; flex-wrap: wrap; }
.key-badge {
    background: #f1f5f9;
    padding: 0.15rem 0.5rem;
    border-radius: 4px;
    font-size: 0.8em;
    font-family: ui-monospace, monospace;
    color: #334155;
}

/* Arrangement Actions */
.arrangement-actions { display: flex; flex-direction: column; align-items: flex-end; gap: 0.5rem; }
.secondary-actions { font-size: 0.85em; color: #64748b; }
.secondary-actions a { color: #64748b; text-decoration: none; }
.secondary-actions a:hover { color: var(--accent); text-decoration: underline; }

/* ── Sidebar & Forms ──────────────────────────────────────────── */
.sidebar-card { background: #fff; border: 1px solid var(--border); border-radius: 8px; padding: 1.5rem; }
.sidebar-card h3 { margin: 0 0 0.5rem 0; font-size: 1.1em; }

/* Practice Form */
.practice-form .input-group { display: flex; align-items: center; gap: 0.5rem; }
.practice-result {
    margin-top: 1rem;
    padding: 0.75rem;
    background: #eff6ff;
    border: 1px solid #bfdbfe;
    border-radius: 6px;
    font-size: 0.9em;
    word-break: break-all;
}
.practice-result a { color: #1d4ed8; font-weight: 600; }

/* History Form */
.history-form { display: flex; flex-direction: column; gap: 0.75rem; margin-bottom: 1.5rem; }
.form-row { display: flex; gap: 0.5rem; }
.form-group { display: flex; flex-direction: column; flex: 1; }
.form-group label { font-size: 0.8em; text-transform: uppercase; letter-spacing: 0.05em; color: #64748b; margin-bottom: 0.25rem; }
.form-group input { width: 100%; padding: 0.4rem; border: 1px solid var(--border); border-radius: 4px; font: inherit; font-size: 0.9em; }
'''
    p.write_text(src)
    print("  ✓ CSS fixed")
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
