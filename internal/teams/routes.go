package teams

import (
	"embed"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
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
		assign:   scheduling.NewAssignmentRepository(db),
		renderer: renderer,
	}}
}

func (m *Module) Routes(r chi.Router, authn *auth.Authenticator) {
	r.Group(func(r chi.Router) {
		r.Use(authn.RequireRoles(auth.RoleAdmin))
		r.Route("/teams", func(r chi.Router) {
			r.Get("/", m.handler.Index)
			r.Post("/", m.handler.Create)
			r.Get("/{id}", m.handler.View)
			r.Post("/{id}/delete", m.handler.Delete)
			r.Post("/{id}/positions", m.handler.CreatePosition)
			r.Post("/{id}/positions/{pid}/delete", m.handler.DeletePosition)
		})
	})
}
