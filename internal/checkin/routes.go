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
		r.Get("/retrieve", m.handler.RetrieveCode)
		r.Get("/family", m.handler.ManageFamily)
		r.Get("/child/{id}/edit", m.handler.EditChildForm)
		r.Post("/child/{id}", m.handler.UpdateChild)
		r.Post("/child/{id}/delete", m.handler.DeleteChild)
		r.Post("/guardian", m.handler.AddGuardian)
		r.Post("/guardian/{id}/delete", m.handler.DeleteGuardian)
	})
}
