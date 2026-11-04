package teams

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
	"github.com/yourchurch/churchplanner/internal/scheduling"
)

type Handler struct {
	assign   *scheduling.AssignmentRepository
	renderer *httpx.Renderer
}

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	teams, err := h.assign.ListTeamsWithCounts(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	h.renderer.Render(w, r, http.StatusOK, "teams_index", map[string]any{
		"Teams": teams,
	})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name != "" {
		_ = h.assign.CreateTeam(r.Context(), name)
	}
	http.Redirect(w, r, "/teams", http.StatusSeeOther)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_ = h.assign.DeleteTeam(r.Context(), id)
	http.Redirect(w, r, "/teams", http.StatusSeeOther)
}

func (h *Handler) View(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Simple way to get team details without adding another repo method
	teams, _ := h.assign.ListTeamsWithCounts(r.Context())
	var teamName string
	for _, t := range teams {
		if t.ID == id {
			teamName = t.Name
			break
		}
	}

	positions, _ := h.assign.ListPositions(r.Context())
	var teamPositions []scheduling.Position
	for _, p := range positions {
		if p.TeamID == id {
			teamPositions = append(teamPositions, p)
		}
	}

	h.renderer.Render(w, r, http.StatusOK, "teams_view", map[string]any{
		"TeamID":    id,
		"TeamName":  teamName,
		"Positions": teamPositions,
	})
}

func (h *Handler) CreatePosition(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	name := strings.TrimSpace(r.FormValue("name"))
	if name != "" {
		_ = h.assign.CreatePosition(r.Context(), teamID, name)
	}
	http.Redirect(w, r, "/teams/"+teamID, http.StatusSeeOther)
}

func (h *Handler) DeletePosition(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	posID := chi.URLParam(r, "pid")
	_ = h.assign.DeletePosition(r.Context(), posID)
	http.Redirect(w, r, "/teams/"+teamID, http.StatusSeeOther)
}
