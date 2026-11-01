package songs

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

type Handler struct {
	svc      *Service
	renderer *httpx.Renderer
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	h.renderer.Render(w, http.StatusOK, "songs_list", map[string]any{"Songs": items})
}

func (h *Handler) New(w http.ResponseWriter, r *http.Request) {
	h.renderer.Render(w, http.StatusOK, "songs_form", map[string]any{"Song": Song{}})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	in := parseInput(r)
	if _, err := h.svc.Create(r.Context(), in); err != nil {
		h.renderer.Render(w, http.StatusUnprocessableEntity, "songs_form", map[string]any{
			"Song":  Song{Title: in.Title, Author: in.Author, Key: in.Key, Lyrics: in.Lyrics, Chords: in.Chords},
			"Error": err.Error(),
		})
		return
	}
	http.Redirect(w, r, "/songs", http.StatusSeeOther)
}

func (h *Handler) Edit(w http.ResponseWriter, r *http.Request) {
	song, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderer.Render(w, http.StatusOK, "songs_form", map[string]any{"Song": song})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if _, err := h.svc.Update(r.Context(), chi.URLParam(r, "id"), parseInput(r)); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/songs", http.StatusSeeOther)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/songs", http.StatusSeeOther)
}

func parseInput(r *http.Request) CreateSongInput {
	tempo, _ := strconv.Atoi(r.FormValue("tempo"))
	return CreateSongInput{
		Title:  strings.TrimSpace(r.FormValue("title")),
		Author: strings.TrimSpace(r.FormValue("author")),
		Key:    strings.TrimSpace(r.FormValue("key")),
		Tempo:  tempo,
		Lyrics: r.FormValue("lyrics"),
		Chords: r.FormValue("chords"),
	}
}
