package people

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
	"github.com/yourchurch/churchplanner/internal/scheduling"
)

type Handler struct {
	repo     *Repository
	schedRepo *scheduling.AssignmentRepository
	renderer *httpx.Renderer
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

	h.renderer.Render(w, r, http.StatusOK, "people_view", map[string]any{
		"User":      user,
		"Positions": positions,
		"SkillMap":  skillMap,
	})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	phone := strings.TrimSpace(r.FormValue("phone"))
	vLow := strings.TrimSpace(r.FormValue("vocal_low"))
	vHigh := strings.TrimSpace(r.FormValue("vocal_high"))
	role := strings.TrimSpace(r.FormValue("role"))

	if err := h.repo.Update(r.Context(), id, phone, vLow, vHigh, role); err != nil {
		httpx.ServerError(w, err)
		return
	}

	positionIDs := r.Form["position_ids"]
	if err := h.repo.SetPositions(r.Context(), id, positionIDs); err != nil {
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

	_, err := h.repo.Create(r.Context(), name, email, password, role)
	if err != nil {
		h.renderer.Render(w, r, http.StatusUnprocessableEntity, "people_form", map[string]any{
			"Error": err.Error(),
			"Name":  name,
			"Email": email,
			"Role":  role,
		})
		return
	}

	// Redirect to the new user's profile to fill out skills
	// (We'd need the ID, but for simplicity we just go back to the list)
	http.Redirect(w, r, "/people", http.StatusSeeOther)
}
