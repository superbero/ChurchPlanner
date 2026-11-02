#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }
if ls migrations/00020_* >/dev/null 2>&1; then
  echo "✗ migrations/00020_* already exists"; exit 1
fi

echo "▸ [1/7] Migration 00020: reorganize teams"
cat > migrations/00020_teams_reorg.sql <<'SQLEOF'
-- +goose Up
-- Rename the two legacy teams to match the church's naming
UPDATE teams SET name = 'Worship Team' WHERE name = 'Worship';
UPDATE teams SET name = 'Media Team'   WHERE name = 'Production';

-- Ensure Sound exists as its own team
INSERT INTO teams (name) VALUES ('Sound')
ON CONFLICT (name) DO NOTHING;

-- Move the existing "Sound" position from Media Team to the Sound team
UPDATE positions
SET team_id = (SELECT id FROM teams WHERE name = 'Sound')
WHERE name = 'Sound'
  AND team_id = (SELECT id FROM teams WHERE name = 'Media Team');

-- +goose Down
UPDATE positions
SET team_id = (SELECT id FROM teams WHERE name = 'Media Team')
WHERE name = 'Sound'
  AND team_id = (SELECT id FROM teams WHERE name = 'Sound');
DELETE FROM teams WHERE name = 'Sound';
UPDATE teams SET name = 'Worship'    WHERE name = 'Worship Team';
UPDATE teams SET name = 'Production' WHERE name = 'Media Team';
SQLEOF
echo "  ✓ migration written"

echo ""
echo "▸ [2/7] Rewrite people/repository.go"
cat > internal/people/repository.go <<'GOEOF'
package people

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

type UserWithSkills struct {
	auth.User
	Skills    string
	TeamRoles string
}

func (r *Repository) List(ctx context.Context) ([]UserWithSkills, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.id, u.name, u.email, u.role, u.phone, u.vocal_low, u.vocal_high,
		       COALESCE((
		           SELECT string_agg(t.name || ' · ' || p.name, ', ' ORDER BY t.name, p.position)
		           FROM user_positions up
		           JOIN positions p ON p.id = up.position_id
		           JOIN teams t ON t.id = p.team_id
		           WHERE up.user_id = u.id
		       ), ''),
		       COALESCE((
		           SELECT string_agg(t.name || ' (' || tm.role || ')', ', ' ORDER BY t.name)
		           FROM team_memberships tm
		           JOIN teams t ON t.id = tm.team_id
		           WHERE tm.user_id = u.id
		       ), '')
		FROM users u
		ORDER BY u.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UserWithSkills
	for rows.Next() {
		var u UserWithSkills
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role,
			&u.Phone, &u.VocalLow, &u.VocalHigh, &u.Skills, &u.TeamRoles); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id string) (auth.User, error) {
	var u auth.User
	err := r.db.QueryRow(ctx, `
		SELECT id, name, email, role, phone, vocal_low, vocal_high
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Phone, &u.VocalLow, &u.VocalHigh)
	return u, err
}

func (r *Repository) Create(ctx context.Context, name, email, password, role string) (string, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return "", err
	}
	var id string
	err = r.db.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, name, email, hash, role).Scan(&id)
	return id, err
}

func (r *Repository) Update(ctx context.Context, id, phone, vocalLow, vocalHigh, role string) error {
	if role == "" {
		role = "volunteer"
	}
	_, err := r.db.Exec(ctx, `
		UPDATE users SET phone = $2, vocal_low = $3, vocal_high = $4, role = $5
		WHERE id = $1
	`, id, phone, vocalLow, vocalHigh, role)
	return err
}

func (r *Repository) PositionsFor(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT position_id FROM user_positions WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *Repository) SetPositions(ctx context.Context, userID string, positionIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM user_positions WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for _, posID := range positionIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO user_positions (user_id, position_id) VALUES ($1, $2)`, userID, posID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

type TeamRole struct {
	TeamID   string
	TeamName string
	Role     string
}

func (r *Repository) TeamRolesFor(ctx context.Context, userID string) ([]TeamRole, error) {
	rows, err := r.db.Query(ctx, `
		SELECT t.id, t.name, tm.role
		FROM team_memberships tm
		JOIN teams t ON t.id = tm.team_id
		WHERE tm.user_id = $1
		ORDER BY t.name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TeamRole
	for rows.Next() {
		var t TeamRole
		if err := rows.Scan(&t.TeamID, &t.TeamName, &t.Role); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
GOEOF
echo "  ✓ repository.go rewritten"

echo ""
echo "▸ [3/7] Rewrite people/handler.go"
cat > internal/people/handler.go <<'GOEOF'
package people

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
	"github.com/yourchurch/churchplanner/internal/scheduling"
)

type Handler struct {
	repo        *Repository
	schedRepo   *scheduling.AssignmentRepository
	memberships *scheduling.TeamMembershipRepository
	renderer    *httpx.Renderer
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.repo.List(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	h.renderer.Render(w, r, http.StatusOK, "people_list", map[string]any{
		"Users": users,
	})
}

func (h *Handler) View(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user, err := h.repo.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	positions, err := h.schedRepo.ListPositions(r.Context())
	if err != nil {
		slog.Error("ListPositions", "err", err)
		httpx.ServerError(w, err)
		return
	}
	teams, err := h.schedRepo.ListTeams(r.Context())
	if err != nil {
		slog.Error("ListTeams", "err", err)
		httpx.ServerError(w, err)
		return
	}

	userSkills, err := h.repo.PositionsFor(r.Context(), id)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	skillMap := map[string]bool{}
	for _, s := range userSkills {
		skillMap[s] = true
	}

	teamRoles, err := h.repo.TeamRolesFor(r.Context(), id)
	if err != nil {
		slog.Error("TeamRolesFor", "err", err)
		httpx.ServerError(w, err)
		return
	}
	membershipRoles := map[string]string{}
	for _, tr := range teamRoles {
		membershipRoles[tr.TeamID] = tr.Role
	}

	h.renderer.Render(w, r, http.StatusOK, "people_view", map[string]any{
		"User":            user,
		"Positions":       positions,
		"SkillMap":        skillMap,
		"Teams":           teams,
		"MembershipRoles": membershipRoles,
	})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	phone := strings.TrimSpace(r.FormValue("phone"))
	vLow := strings.TrimSpace(r.FormValue("vocal_low"))
	vHigh := strings.TrimSpace(r.FormValue("vocal_high"))
	userRole := strings.TrimSpace(r.FormValue("role"))

	if err := h.repo.Update(r.Context(), id, phone, vLow, vHigh, userRole); err != nil {
		httpx.ServerError(w, err)
		return
	}

	positionIDs := r.Form["position_ids"]
	if err := h.repo.SetPositions(r.Context(), id, positionIDs); err != nil {
		httpx.ServerError(w, err)
		return
	}

	memberships := map[string]string{}
	for key, values := range r.Form {
		if !strings.HasPrefix(key, "team_role_") || len(values) == 0 {
			continue
		}
		teamRole := strings.TrimSpace(values[0])
		if teamRole == "" {
			continue
		}
		teamID := strings.TrimPrefix(key, "team_role_")
		memberships[teamID] = teamRole
	}
	if err := h.memberships.SetForUser(r.Context(), id, memberships); err != nil {
		httpx.ServerError(w, err)
		return
	}

	http.Redirect(w, r, "/people/"+id, http.StatusSeeOther)
}

func (h *Handler) New(w http.ResponseWriter, r *http.Request) {
	h.renderer.Render(w, r, http.StatusOK, "people_form", nil)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	role := strings.TrimSpace(r.FormValue("role"))
	if role == "" {
		role = "volunteer"
	}

	if _, err := h.repo.Create(r.Context(), name, email, password, role); err != nil {
		h.renderer.Render(w, r, http.StatusUnprocessableEntity, "people_form", map[string]any{
			"Error": err.Error(), "Name": name, "Email": email, "Role": role,
		})
		return
	}
	http.Redirect(w, r, "/people", http.StatusSeeOther)
}
GOEOF
echo "  ✓ handler.go rewritten"

echo ""
echo "▸ [4/7] Rewrite people/routes.go"
cat > internal/people/routes.go <<'GOEOF'
package people

import (
	"embed"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
	"github.com/yourchurch/churchplanner/internal/scheduling"
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
	return &Module{handler: &Handler{
		repo:        NewRepository(db),
		schedRepo:   scheduling.NewAssignmentRepository(db),
		memberships: scheduling.NewTeamMembershipRepository(db),
		renderer:    renderer,
	}}
}

func (m *Module) Routes(r chi.Router) {
	r.Route("/people", func(r chi.Router) {
		r.Get("/", m.handler.List)
		r.Get("/new", m.handler.New)
		r.Post("/", m.handler.Create)
		r.Get("/{id}", m.handler.View)
		r.Post("/{id}", m.handler.Update)
	})
}
GOEOF
echo "  ✓ routes.go rewritten"

echo ""
echo "▸ [5/7] Rewrite people/templates/list.html"
cat > internal/people/templates/list.html <<'HTMLEOF'
{{define "content"}}
<h1>People</h1>
<p class="muted">Directory of volunteers and staff.</p>
<p><a class="button" href="/people/new">Add new person</a></p>

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Email</th>
            <th>Phone</th>
            <th>Vocal Range</th>
            <th>Team(s)</th>
            <th>Skills / Instruments</th>
            <th></th>
        </tr>
    </thead>
    <tbody>
    {{range .Users}}
        <tr>
            <td><a href="/people/{{.ID}}"><strong>{{.Name}}</strong></a> <span class="chip">{{.Role}}</span></td>
            <td>{{.Email}}</td>
            <td>{{.Phone}}</td>
            <td>{{if .VocalLow}}{{.VocalLow}} – {{.VocalHigh}}{{end}}</td>
            <td class="team-cell">{{if .TeamRoles}}{{.TeamRoles}}{{else}}<span class="muted">—</span>{{end}}</td>
            <td class="skills-cell">{{if .Skills}}{{.Skills}}{{else}}<span class="muted">—</span>{{end}}</td>
            <td><a href="/people/{{.ID}}">Edit</a></td>
        </tr>
    {{else}}
        <tr><td colspan="7">No people found.</td></tr>
    {{end}}
    </tbody>
</table>
{{end}}
HTMLEOF
echo "  ✓ list.html rewritten"

echo ""
echo "▸ [6/7] Rewrite people/templates/view.html"
cat > internal/people/templates/view.html <<'HTMLEOF'
{{define "content"}}
<p><a href="/people">← All people</a></p>
<h1>{{.User.Name}}</h1>

<div class="profile-grid">
    <form method="post" action="/people/{{.User.ID}}" class="profile-form">

        <h2>Account</h2>
        <label>Email <input value="{{.User.Email}}" disabled></label>
        <label>Role
            <select name="role">
                <option value="volunteer" {{if eq .User.Role "volunteer"}}selected{{end}}>Volunteer (Scheduled Viewer)</option>
                <option value="guest"     {{if eq .User.Role "guest"}}selected{{end}}>Guest</option>
                <option value="editor"    {{if eq .User.Role "editor"}}selected{{end}}>Editor</option>
                <option value="admin"     {{if eq .User.Role "admin"}}selected{{end}}>Admin</option>
            </select>
        </label>
        <label>Phone <input name="phone" value="{{.User.Phone}}" placeholder="555-0100"></label>
        <div class="row">
            <label>Vocal Low <input name="vocal_low" value="{{.User.VocalLow}}" size="4" placeholder="A3"></label>
            <label>Vocal High <input name="vocal_high" value="{{.User.VocalHigh}}" size="4" placeholder="E5"></label>
        </div>

        <h2>Skills / Instruments</h2>
        <p class="muted">Add the positions this person is comfortable filling.</p>

        <div id="active-skills" class="active-skills-panel">
            {{range .Positions}}
                {{if index $.SkillMap .ID}}
                <div class="skill-chip" data-id="{{.ID}}" data-team="{{.TeamName}}" data-name="{{.Name}}">
                    <span><strong>{{.TeamName}}</strong> · {{.Name}}</span>
                    <button type="button" class="remove-skill" title="Remove">×</button>
                    <input type="hidden" name="position_ids" value="{{.ID}}">
                </div>
                {{end}}
            {{end}}
        </div>

        <div class="add-skill-row">
            <select id="skill-picker">
                <option value="">— Select a skill to add —</option>
                {{range .Positions}}
                    {{if not (index $.SkillMap .ID)}}
                    <option value="{{.ID}}" data-team="{{.TeamName}}" data-name="{{.Name}}">{{.TeamName}} · {{.Name}}</option>
                    {{end}}
                {{end}}
            </select>
            <button type="button" id="add-skill-btn" class="button-secondary">Add</button>
        </div>

        <h2>Team Memberships</h2>
        <p class="muted">Leaders can manage plans for their team without admin access to People or Reports.</p>
        <div class="team-memberships">
            {{range .Teams}}
            {{$currentRole := index $.MembershipRoles .ID}}
            <div class="membership-row">
                <span class="membership-name">{{.Name}}</span>
                <select name="team_role_{{.ID}}" class="membership-select">
                    <option value=""       {{if eq $currentRole ""}}selected{{end}}>— not a member —</option>
                    <option value="member" {{if eq $currentRole "member"}}selected{{end}}>Member</option>
                    <option value="leader" {{if eq $currentRole "leader"}}selected{{end}}>Leader (can manage plans)</option>
                </select>
            </div>
            {{else}}
            <p class="muted">No teams configured.</p>
            {{end}}
        </div>

        <p style="margin-top:1.5rem">
            <button type="submit">Save Profile</button>
        </p>
    </form>
</div>
{{end}}
HTMLEOF
echo "  ✓ view.html rewritten"

echo ""
echo "▸ [7/7] CSS + cache bump + build"
python3 - <<'PYEOF'
from pathlib import Path
import re

p = Path("internal/web/static/css/app.css")
src = p.read_text()
if '.team-memberships' not in src:
    src += '''

/* Team memberships */
.team-memberships {
    background: #fff;
    padding: 1rem 1.25rem;
    border: 1px solid var(--border);
    border-radius: 8px;
    margin-bottom: 1rem;
}
.membership-row {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: .5rem 0;
    border-bottom: 1px solid #f0f3f7;
}
.membership-row:last-child { border-bottom: 0; }
.membership-name { font-weight: 600; min-width: 10rem; }
.membership-select {
    flex: 1;
    max-width: 20rem;
    padding: .35rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    font: inherit;
}

/* People list Teams column */
.team-cell {
    font-size: 0.9em;
    color: #475467;
    max-width: 250px;
    line-height: 1.4;
}
'''
    p.write_text(src)
    print("  ✓ CSS appended")

p2 = Path("internal/web/templates/layout.html")
src2 = p2.read_text()
m = re.search(r'app\.css\?v=(\d+)', src2)
if m:
    src2 = src2.replace(m.group(0), f'app.css?v={int(m.group(1))+1}')
    p2.write_text(src2)
    print("  ✓ cache bumped")
PYEOF

if ! go build ./...; then
    echo "✗ build failed — paste errors"
    exit 1
fi
echo "✓ build OK"

docker compose -f deploy/docker-compose.yml up -d --build

echo ""
echo "▸ Logs (15s)"
timeout 15 docker compose -f deploy/docker-compose.yml logs -f app 2>&1 || true

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  Verify"
echo "════════════════════════════════════════════════════════════════"
cat <<'CHECKLIST'
  1. HARD-REFRESH the browser.

  2. /people:
     → New "Team(s)" column in the table.
     → Admins show "—", will populate once you assign teams.

  3. Open Jane Doe:
     → "Team Memberships" now shows FOUR teams:
         Worship Team, Media Team, Sound, Kids
     → Each has a dropdown: not a member / member / leader.
     → Set Worship Team = "Leader", Save Profile.

  4. Back to /people:
     → Jane's Team(s) cell shows "Worship Team (leader)".

  5. Log out, log in as Jane:
     → Lands on /plans.
     → Assign dropdown shows only Worship Team positions.
     → Production/Media and Sound positions are hidden.
     → Publish/Delete footer is gone.

  6. Try direct URL manipulation:
     → POST to /plans/<id>/people with a Media Team position_id as Jane
     → 403 forbidden from the server.
CHECKLIST
