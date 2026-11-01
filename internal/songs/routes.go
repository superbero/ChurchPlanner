package songs

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
	return &Module{handler: &Handler{
		svc:      NewService(NewRepository(db)),
		renderer: renderer,
	}}
}

func (m *Module) Routes(r chi.Router) {
	r.Route("/songs", func(r chi.Router) {
		r.Get("/", m.handler.List)
		r.Get("/new", m.handler.New)
		r.Post("/", m.handler.Create)
		r.Get("/{id}/edit", m.handler.Edit)
		r.Post("/{id}", m.handler.Update)
		r.Post("/{id}/delete", m.handler.Delete)
	})
}
