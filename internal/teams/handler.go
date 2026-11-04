package teams

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

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
	if name == "" {
		http.Error(w, "team name is required", http.StatusBadRequest)
		return
	}
	if err := h.assign.CreateTeam(r.Context(), name); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/teams", http.StatusSeeOther)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.assign.DeleteTeam(r.Context(), id); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/teams", http.StatusSeeOther)
}

func (h *Handler) View(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	teams, err := h.assign.ListTeamsWithCounts(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	var teamName string
	for _, t := range teams {
		if t.ID == id {
			teamName = t.Name
			break
		}
	}
	if teamName == "" {
		http.NotFound(w, r)
		return
	}

	positions, err := h.assign.ListPositions(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
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
	if name == "" {
		http.Error(w, "position name is required", http.StatusBadRequest)
		return
	}
	if err := h.assign.CreatePosition(r.Context(), teamID, name); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/teams/"+teamID, http.StatusSeeOther)
}

func (h *Handler) DeletePosition(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	posID := chi.URLParam(r, "pid")
	if err := h.assign.DeletePositionForTeam(r.Context(), teamID, posID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/teams/"+teamID, http.StatusSeeOther)
}
