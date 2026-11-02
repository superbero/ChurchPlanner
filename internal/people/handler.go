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
