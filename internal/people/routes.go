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
		repo:      NewRepository(db),
		schedRepo: scheduling.NewAssignmentRepository(db),
		renderer:  renderer,
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
