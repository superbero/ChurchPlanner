#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  1/6  Update Renderer to inject CurrentUser everywhere"
echo "════════════════════════════════════════════════════════════════"
cat > internal/platform/httpx/render.go <<'GOEOF'
package httpx

import (
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
)

func funcMap() template.FuncMap {
	return template.FuncMap{
		"contains":   strings.Contains,
		"hasPrefix":  strings.HasPrefix,
		"hasSuffix":  strings.HasSuffix,
		"split":      strings.Split,
		"splitAfter": strings.SplitAfter,
		"trimPrefix": strings.TrimPrefix,
		"trimSuffix": strings.TrimSuffix,
		"lower":      strings.ToLower,
		"upper":      strings.ToUpper,
		"languageLabel":   languageLabel,
		"languageOptions": languageOptions,
	}
}

type LanguageOption struct {
	Code  string
	Label string
}

var languageLabels = map[string]string{
	"en": "English", "fr": "Français", "es": "Español", "pt": "Português",
	"de": "Deutsch", "it": "Italiano", "nl": "Nederlands", "sw": "Kiswahili",
	"zh": "中文", "ko": "한국어",
}
var languageOrder = []string{"en", "fr", "es", "pt", "de", "it", "nl", "sw", "zh", "ko"}

func languageLabel(code string) string {
	if code == "" { return "" }
	if l, ok := languageLabels[code]; ok { return l }
	return strings.ToUpper(code)
}

func languageOptions() []LanguageOption {
	out := make([]LanguageOption, 0, len(languageOrder))
	for _, c := range languageOrder {
		out = append(out, LanguageOption{Code: c, Label: languageLabels[c]})
	}
	return out
}

type Renderer struct {
	layout *template.Template
	pages  map[string]*template.Template
}

func NewRenderer(layoutFS fs.FS) (*Renderer, error) {
	layout, err := template.New("layout.html").Funcs(funcMap()).ParseFS(layoutFS, "layout.html")
	if err != nil {
		return nil, fmt.Errorf("parse layout: %w", err)
	}
	return &Renderer{layout: layout, pages: make(map[string]*template.Template)}, nil
}

func (r *Renderer) Add(prefix string, fsys fs.FS, glob string) error {
	matches, err := fs.Glob(fsys, glob)
	if err != nil { return err }
	for _, m := range matches {
		t, err := r.layout.Clone()
		if err != nil { return err }
		if _, err := t.ParseFS(fsys, m); err != nil {
			return fmt.Errorf("parse %s: %w", m, err)
		}
		name := prefix + "_" + strings.TrimSuffix(path.Base(m), ".html")
		r.pages[name] = t
	}
	return nil
}

// Render now requires the request so it can inject CurrentUser into the template.
func (r *Renderer) Render(w http.ResponseWriter, req *http.Request, status int, page string, data any) {
	t, ok := r.pages[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}

	// Ensure data is a map so we can inject the user
	var dataMap map[string]any
	if data == nil {
		dataMap = make(map[string]any)
	} else if m, ok := data.(map[string]any); ok {
		dataMap = m
	} else {
		http.Error(w, "renderer requires map[string]any", http.StatusInternalServerError)
		return
	}

	if u, ok := auth.UserFrom(req.Context()); ok {
		dataMap["CurrentUser"] = u
	} else {
		dataMap["CurrentUser"] = nil
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.ExecuteTemplate(w, "layout.html", dataMap); err != nil {
		slog.Error("render", "page", page, "err", err)
	}
}

func ServerError(w http.ResponseWriter, err error) {
	slog.Error("handler error", "err", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
GOEOF

# Fix all handler Render calls across the app (regex replace)
echo "  ✓ renderer updated"
python3 - <<'PYEOF'
import re
from pathlib import Path

for p in Path("internal").rglob("*.go"):
    if p.name.endswith("_test.go") or "render.go" in p.name: continue
    src = p.read_text()
    # Find .Render(w, http.Status...
    # Replace with .Render(w, r, http.Status...
    new_src = re.sub(r'\.Render\(w,\s*(http\.Status)', r'.Render(w, r, \1', src)
    if new_src != src:
        p.write_text(new_src)
        print(f"  · updated {p}")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  2/6  Layout: Dynamic Nav based on Role"
echo "════════════════════════════════════════════════════════════════"
cat > internal/web/templates/layout.html <<'HTMLEOF'
{{define "layout.html"}}
<!doctype html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>{{block "title" .}}Church Planner{{end}}</title>
    <link rel="stylesheet" href="/static/css/app.css?v=10">
    <script src="/static/js/app.js?v=10" defer></script>
</head>
<body>
    <header class="nav">
        <a href="/" class="brand">Church Planner</a>
        <nav>
            {{if .CurrentUser}}
                {{if eq .CurrentUser.Role "admin"}}
                    <a href="/plans">Plans</a>
                    <a href="/people">People</a>
                    <a href="/songs">Songs</a>
                    <a href="/calendar">Calendar</a>
                    <a href="/reports/ccli">Reports</a>
                {{else}}
                    <a href="/my-schedule">My Schedule</a>
                    <a href="/calendar">Calendar</a>
                    <a href="/songs">Songs</a>
                {{end}}
            {{end}}
        </nav>
        {{if .CurrentUser}}
        <div class="user-menu">
            <span class="user-name">{{.CurrentUser.Name}}</span>
            <form method="post" action="/logout" class="logout"><button>Sign out</button></form>
        </div>
        {{end}}
    </header>
    <main>
        {{template "content" .}}
    </main>
</body>
</html>
{{end}}
HTMLEOF
echo "  ✓ layout updated"

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  3/6  Router: Group routes by Role + Add Dashboard"
echo "════════════════════════════════════════════════════════════════"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/web/router.go")
src = p.read_text()

# 1. Update Login redirect to go to / instead of /songs
src = src.replace(
    'http.Redirect(w, r, "/songs", http.StatusSeeOther)\n\t})\n\tr.Post("/logout"',
    'http.Redirect(w, r, "/", http.StatusSeeOther)\n\t})\n\tr.Post("/logout"'
)

# 2. Rewrite the route grouping
start = src.find('\t// --- modules: construct once, mount once ---')
if start == -1:
    print("  ! modules mount point not found")
    raise SystemExit(1)

new_routes = '''	// --- modules: construct once, mount once ---
	songsModule := songs.New(d.DB, renderer, songs.HandlerConfig{
		PublicBaseURL:   d.Config.PublicBaseURL,
		TranslateAPIURL: d.Config.TranslateAPIURL,
		TranslateAPIKey: d.Config.TranslateAPIKey,
		PrimaryLanguage: d.Config.PrimaryLanguage,
	})
	schedulingModule := scheduling.New(d.DB, renderer)

	songsModule.PublicRoutes(r)

	r.Group(func(r chi.Router) {
		r.Use(authn.RequireUser)

		// Dashboard (redirects based on role)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			u, _ := auth.UserFrom(r.Context())
			if u.Role == "admin" {
				http.Redirect(w, r, "/plans", http.StatusSeeOther)
			} else {
				http.Redirect(w, r, "/my-schedule", http.StatusSeeOther)
			}
		})

		// --- Shared (Volunteer + Admin) ---
		schedulingModule.MyScheduleRoutes(r)
		calendar.New(d.DB, renderer).Routes(r)

		// Songs (Volunteers can view/list, but Edit buttons will be handled in templates)
		songsModule.Routes(r)

		// --- Admin Only ---
		r.Group(func(r chi.Router) {
			r.Use(authn.RequireRole("admin"))

			schedulingModule.Routes(r) // Plans, Times, Items, Assignments
			people.New(d.DB, renderer).Routes(r)
			reports.New(d.DB, renderer).Routes(r)
		})
	})

	return r, nil
}'''

src = src[:start] + new_routes
p.write_text(src)
print("  ✓ router updated with Admin/Volunteer groups")
PYEOF

# We need to split MyScheduleRoutes out of scheduling routes
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/scheduling/routes.go")
src = p.read_text()

if 'func (m *Module) MyScheduleRoutes' not in src:
    # 1. Remove my-schedule from Routes()
    src = src.replace(
        '''	// Volunteer self-service
	r.Get("/my-schedule", m.handler.MySchedule)
	r.Post("/my-schedule/{ppid}/respond", m.handler.RespondToAssignment)''',
        ''
    )

    # 2. Add it as a new method
    src += '''

// MyScheduleRoutes are accessible to everyone.
func (m *Module) MyScheduleRoutes(r chi.Router) {
	r.Get("/my-schedule", m.handler.MySchedule)
	r.Post("/my-schedule/{ppid}/respond", m.handler.RespondToAssignment)
}
'''
    p.write_text(src)
    print("  ✓ scheduling routes split")
else:
    print("  · scheduling routes already split")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  4/6  Templates: Hide Admin Buttons from Volunteers"
echo "════════════════════════════════════════════════════════════════"
python3 - <<'PYEOF'
from pathlib import Path

# Songs list: Hide "Add song" and "Import" for volunteers
p = Path("internal/songs/templates/list.html")
src = p.read_text()
if '{{if eq .CurrentUser.Role "admin"}}' not in src:
    src = src.replace(
        '<p>\n    <a class="button" href="/songs/new">Add song</a>',
        '<p>\n    {{if eq .CurrentUser.Role "admin"}}\n    <a class="button" href="/songs/new">Add song</a>'
    )
    src = src.replace(
        '<a class="button-secondary" href="/songs/tags">Manage tags</a>',
        '<a class="button-secondary" href="/songs/tags">Manage tags</a>\n    {{end}}'
    )
    p.write_text(src)
    print("  ✓ songs list locked down")

# Song detail: Hide edit/delete for volunteers
p = Path("internal/songs/templates/detail.html")
src = p.read_text()
if '{{if eq .CurrentUser.Role "admin"}}' not in src:
    # Hide main edit button
    src = src.replace(
        '<a class="button-secondary" href="/songs/{{.Song.ID}}/edit">Edit metadata</a>',
        '{{if eq .CurrentUser.Role "admin"}}<a class="button-secondary" href="/songs/{{.Song.ID}}/edit">Edit metadata</a>{{end}}'
    )
    # Hide add arrangement button
    src = src.replace(
        '<a class="button" href="/songs/{{.Song.ID}}/arrangements/new">Add arrangement</a>',
        '{{if eq .CurrentUser.Role "admin"}}<a class="button" href="/songs/{{.Song.ID}}/arrangements/new">Add arrangement</a>{{end}}'
    )
    p.write_text(src)
    print("  ✓ song detail locked down")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  5/6  CSS for user menu"
echo "════════════════════════════════════════════════════════════════"
cat >> internal/web/static/css/app.css <<'CSSEOF'

/* User Menu */
.user-menu { margin-left: auto; display: flex; align-items: center; gap: 1rem; }
.user-name { font-size: 0.9em; color: #666; font-weight: 500; }
.nav nav a { color: #475467; font-weight: 500; }
.nav nav a:hover { color: var(--accent); }
CSSEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  6/6  Build & Restart"
echo "════════════════════════════════════════════════════════════════"
if ! go build ./...; then
    echo "✗ build failed — paste errors"
    exit 1
fi
echo "✓ build OK"

docker compose -f deploy/docker-compose.yml up -d --build
echo ""
echo "✅ Done."
echo "Log out and log back in to see the new role-based UI."
