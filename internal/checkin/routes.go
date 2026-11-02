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

// PublicRoutes are available to parents on a lobby kiosk without login.
func (m *Module) PublicRoutes(r chi.Router) {
	r.Get("/checkin", m.handler.Index)
	r.Get("/checkin/add", m.handler.AddChildForm)
	r.Post("/checkin/add", m.handler.AddChild)
	r.Post("/checkin/submit", m.handler.Submit)
	r.Get("/checkin/retrieve", m.handler.RetrieveCode)
}

// StaffRoutes are for logged-in admins/volunteers to manage family data.
func (m *Module) StaffRoutes(r chi.Router) {
	r.Get("/checkin/family", m.handler.ManageFamily)
	r.Get("/checkin/child/{id}/edit", m.handler.EditChildForm)
	r.Post("/checkin/child/{id}", m.handler.UpdateChild)
	r.Post("/checkin/child/{id}/delete", m.handler.DeleteChild)
	r.Post("/checkin/guardian", m.handler.AddGuardian)
	r.Post("/checkin/guardian/{id}/delete", m.handler.DeleteGuardian)
}
