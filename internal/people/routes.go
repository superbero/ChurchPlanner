package people

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

func New(db *pgxpool.Pool, renderer *httpx.Renderer, resetMailer passwordResetMailer, publicBaseURL string) *Module {
	return &Module{handler: &Handler{
		repo:          NewRepository(db),
		schedRepo:     scheduling.NewAssignmentRepository(db),
		memberships:   scheduling.NewTeamMembershipRepository(db),
		renderer:      renderer,
		mailer:        resetMailer,
		publicBaseURL: publicBaseURL,
	}}
}

func (m *Module) PublicRoutes(r chi.Router) {
	r.Get("/reset-password", m.handler.ShowPasswordReset)
	r.Post("/reset-password", m.handler.CompletePasswordReset)
}

func (m *Module) Routes(r chi.Router, authn *auth.Authenticator) {
	r.Group(func(r chi.Router) {
		r.Use(authn.RequireRoles(auth.RoleManager))
		r.Route("/people", func(r chi.Router) {
			r.Get("/", m.handler.List)
			r.Get("/new", m.handler.New)
			r.Get("/bulk", m.handler.ShowBulkImport)
			r.Post("/bulk", m.handler.BulkImport)
			r.Post("/", m.handler.Create)
			r.Get("/{id}", m.handler.View)
			r.Post("/{id}", m.handler.Update)
			r.Post("/{id}/password-reset", m.handler.ResetPassword)
		})
	})
}
