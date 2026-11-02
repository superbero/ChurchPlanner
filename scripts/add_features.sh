#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }
if ls migrations/00017_* >/dev/null 2>&1; then
  echo "✗ migrations/00017_* already exists"; exit 1
fi

echo "════════════════════════════════════════════════════════════════"
echo "  1/4  Migration 00017: Children & Checkins"
echo "════════════════════════════════════════════════════════════════"
cat > migrations/00017_checkin.sql <<'SQLEOF'
-- +goose Up
CREATE TABLE children (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_phone TEXT NOT NULL,
    first_name   TEXT NOT NULL,
    last_name    TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX children_phone_idx ON children (parent_phone);

CREATE TABLE checkins (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    child_id        UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    security_code   TEXT NOT NULL,
    checked_in_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    checked_out_at  TIMESTAMPTZ
);
CREATE INDEX checkins_child_idx ON checkins (child_id, checked_in_at DESC);

-- +goose Down
DROP TABLE IF EXISTS checkins;
DROP TABLE IF EXISTS children;
SQLEOF
echo "  ✓ migration written"

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  2/4  Checkin Module: internal/checkin"
echo "════════════════════════════════════════════════════════════════"
mkdir -p internal/checkin/templates

cat > internal/checkin/handler.go <<'GOEOF'
package checkin

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

type Handler struct {
	db       *pgxpool.Pool
	renderer *httpx.Renderer
}

type Child struct {
	ID        string
	FirstName string
	LastName  string
	CheckedIn bool
}

// generateCode returns a random string like "X7F-382"
func generateCode() string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const numbers = "0123456789"

	l1, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
	l2, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
	l3, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
	n1, _ := rand.Int(rand.Reader, big.NewInt(int64(len(numbers))))
	n2, _ := rand.Int(rand.Reader, big.NewInt(int64(len(numbers))))
	n3, _ := rand.Int(rand.Reader, big.NewInt(int64(len(numbers))))

	return fmt.Sprintf("%c%c%c-%c%c%c",
		letters[l1.Int64()], letters[l2.Int64()], letters[l3.Int64()],
		numbers[n1.Int64()], numbers[n2.Int64()], numbers[n3.Int64()])
}

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	phone := strings.TrimSpace(r.URL.Query().Get("phone"))

	if phone == "" {
		h.renderer.Render(w, r, http.StatusOK, "checkin_index", nil)
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT id, first_name, last_name FROM children WHERE parent_phone = $1 ORDER BY first_name
	`, phone)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	defer rows.Close()

	var children []Child
	for rows.Next() {
		var c Child
		if err := rows.Scan(&c.ID, &c.FirstName, &c.LastName); err != nil {
			httpx.ServerError(w, err)
			return
		}
		children = append(children, c)
	}

	h.renderer.Render(w, r, http.StatusOK, "checkin_family", map[string]any{
		"Phone":    phone,
		"Children": children,
	})
}

func (h *Handler) AddChildForm(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	h.renderer.Render(w, r, http.StatusOK, "checkin_add", map[string]any{"Phone": phone})
}

func (h *Handler) AddChild(w http.ResponseWriter, r *http.Request) {
	phone := r.FormValue("phone")
	first := strings.TrimSpace(r.FormValue("first_name"))
	last := strings.TrimSpace(r.FormValue("last_name"))

	_, err := h.db.Exec(r.Context(), `
		INSERT INTO children (parent_phone, first_name, last_name) VALUES ($1, $2, $3)
	`, phone, first, last)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	http.Redirect(w, r, "/checkin?phone="+phone, http.StatusSeeOther)
}

func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpx.ServerError(w, err)
		return
	}

	childIDs := r.Form["child_ids"]
	if len(childIDs) == 0 {
		http.Redirect(w, r, "/checkin", http.StatusSeeOther)
		return
	}

	code := generateCode()

	// In a real app, we'd use a transaction. Here, a loop is fine for a small church.
	for _, cid := range childIDs {
		_, err := h.db.Exec(r.Context(), `
			INSERT INTO checkins (child_id, security_code) VALUES ($1, $2)
		`, cid, code)
		if err != nil {
			httpx.ServerError(w, err)
			return
		}
	}

	// Fetch the children for the success tag
	rows, err := h.db.Query(r.Context(), `
		SELECT first_name, last_name FROM children WHERE id = ANY($1::uuid[])
	`, childIDs)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	defer rows.Close()

	var kids []Child
	for rows.Next() {
		var c Child
		rows.Scan(&c.ID, &c.FirstName, &c.LastName)
		kids = append(kids, c)
	}

	h.renderer.Render(w, r, http.StatusOK, "checkin_success", map[string]any{
		"Code":  code,
		"Kids":  kids,
		"Now":   time.Now(),
	})
}
GOEOF

cat > internal/checkin/routes.go <<'GOEOF'
package checkin

import (
	"embed"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

//go:embed templates/*.html
var templatesFS embed.FS

func Templates() fs.FS {
	sub, err := fs.Sub(templatesFS, "templates")
	if err != nil {
		panic(err)
	}
	return sub
}

type Module struct{ handler *Handler }

func New(db *pgxpool.Pool, renderer *httpx.Renderer) *Module {
	return &Module{handler: &Handler{db: db, renderer: renderer}}
}

func (m *Module) Routes(r chi.Router) {
	r.Route("/checkin", func(r chi.Router) {
		r.Get("/", m.handler.Index)
		r.Get("/add", m.handler.AddChildForm)
		r.Post("/add", m.handler.AddChild)
		r.Post("/submit", m.handler.Submit)
	})
}
GOEOF

cat > internal/checkin/templates/index.html <<'HTMLEOF'
{{define "content"}}
<div class="kiosk-container">
    <h1>Kids Check-in</h1>
    <p class="muted">Enter your phone number to begin.</p>
    <form method="get" action="/checkin" class="kiosk-form">
        <input type="tel" name="phone" placeholder="555-0100" required autofocus>
        <button type="submit">Find Family</button>
    </form>
</div>
{{end}}
HTMLEOF

cat > internal/checkin/templates/family.html <<'HTMLEOF'
{{define "content"}}
<div class="kiosk-container">
    <h1>Check-in</h1>
    <p class="muted">Phone: {{.Phone}}</p>

    {{if .Children}}
    <form method="post" action="/checkin/submit" class="kiosk-form">
        <div class="kid-list">
            {{range .Children}}
            <label class="kid-checkbox">
                <input type="checkbox" name="child_ids" value="{{.ID}}" checked>
                <span>{{.FirstName}} {{.LastName}}</span>
            </label>
            {{end}}
        </div>
        <button type="submit" class="kiosk-btn">Check In Selected</button>
    </form>
    {{else}}
    <p>No children found for this number.</p>
    <a href="/checkin/add?phone={{.Phone}}" class="kiosk-btn" style="text-decoration:none">Add a Child</a>
    {{end}}

    <p style="margin-top:2rem"><a href="/checkin">Cancel</a></p>
</div>
{{end}}
HTMLEOF

cat > internal/checkin/templates/add.html <<'HTMLEOF'
{{define "content"}}
<div class="kiosk-container">
    <h1>Add Child</h1>
    <p class="muted">Phone: {{.Phone}}</p>
    <form method="post" action="/checkin/add" class="kiosk-form">
        <input type="hidden" name="phone" value="{{.Phone}}">
        <input type="text" name="first_name" placeholder="First Name" required>
        <input type="text" name="last_name" placeholder="Last Name" required>
        <button type="submit" class="kiosk-btn">Save Child</button>
    </form>
    <p style="margin-top:2rem"><a href="/checkin?phone={{.Phone}}">Cancel</a></p>
</div>
{{end}}
HTMLEOF

cat > internal/checkin/templates/success.html <<'HTMLEOF'
{{define "content"}}
<div class="kiosk-container" style="text-align: center;">
    <h1>✅ Checked In</h1>
    <div class="security-code">{{.Code}}</div>
    <p>Keep this code to pick up your children.</p>

    <div style="margin-top: 2rem; border-top: 1px dashed #ccc; padding-top: 1rem;">
        {{range .Kids}}
        <p class="tag-kid">{{.FirstName}} {{.LastName}}</p>
        {{end}}
    </div>

    <button onclick="window.print()" class="kiosk-btn" style="margin-top:2rem">Print Tags</button>
    <p style="margin-top:1rem"><a href="/checkin">Done</a></p>
</div>
{{end}}
HTMLEOF

echo "  ✓ checkin module created"

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  3/4  Wire into Router & add Kiosk CSS"
echo "════════════════════════════════════════════════════════════════"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/web/router.go")
src = p.read_text()

# Add import
if 'internal/checkin"' not in src:
    src = src.replace(
        '"github.com/yourchurch/churchplanner/internal/calendar"',
        '"github.com/yourchurch/churchplanner/internal/calendar"\n\t"github.com/yourchurch/churchplanner/internal/checkin"'
    )

# Add templates
if '{"checkin", checkin.Templates()}' not in src:
    src = src.replace(
        '{"reports", reports.Templates()},',
        '{"reports", reports.Templates()},\n\t\t{"checkin", checkin.Templates()},'
    )

# Add routes inside the Shared group (Volunteers and Admins can use kiosks)
if 'checkin.New(d.DB, renderer).Routes(r)' not in src:
    src = src.replace(
        'calendar.New(d.DB, renderer).Routes(r)',
        'calendar.New(d.DB, renderer).Routes(r)\n\t\tcheckin.New(d.DB, renderer).Routes(r)'
    )

p.write_text(src)
print("  ✓ router updated")
PYEOF

cat >> internal/web/static/css/app.css <<'CSSEOF'

/* Kiosk Check-in */
.kiosk-container {
    max-width: 600px;
    margin: 3rem auto;
    background: #fff;
    padding: 3rem;
    border-radius: 12px;
    box-shadow: 0 10px 25px rgba(0,0,0,0.05);
}
.kiosk-container h1 { font-size: 2.5rem; margin-top: 0; text-align: center; }
.kiosk-form { display: flex; flex-direction: column; gap: 1.5rem; margin-top: 2rem; }
.kiosk-form input[type="tel"],
.kiosk-form input[type="text"] {
    font-size: 1.5rem;
    padding: 1rem;
    border: 2px solid var(--border);
    border-radius: 8px;
    text-align: center;
}
.kiosk-btn {
    display: block;
    width: 100%;
    font-size: 1.5rem;
    padding: 1rem;
    background: var(--accent);
    color: #fff;
    border: 0;
    border-radius: 8px;
    cursor: pointer;
    text-align: center;
}
.kid-checkbox {
    display: flex;
    align-items: center;
    gap: 1rem;
    font-size: 1.5rem;
    padding: 1rem;
    background: #f7f9fc;
    border-radius: 8px;
    cursor: pointer;
}
.kid-checkbox input { width: 2rem; height: 2rem; }
.security-code {
    font-family: ui-monospace, monospace;
    font-size: 4rem;
    font-weight: bold;
    color: var(--accent);
    letter-spacing: 0.1em;
    margin: 2rem 0;
}
.tag-kid { font-size: 1.25rem; font-weight: bold; }
CSSEOF

echo "  ✓ CSS added"

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  4/4  Build & Restart"
echo "════════════════════════════════════════════════════════════════"
if ! go build ./...; then
    echo "✗ build failed — paste errors"
    exit 1
fi
echo "✓ build OK"

docker compose -f deploy/docker-compose.yml up -d --build
echo ""
echo "✅ Done."
echo "Navigate to http://localhost:8080/checkin to test."
