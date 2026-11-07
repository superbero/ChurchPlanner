package calendar

import (
	"embed"
	"io/fs"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
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

type Module struct {
	handler *Handler
	feed    *FeedHandler
}

func New(db *pgxpool.Pool, renderer *httpx.Renderer) *Module {
	baseURL := os.Getenv("PUBLIC_BASE_URL")
	feed := NewFeedHandler(db, renderer, NewPGFeedSource(db, baseURL), baseURL)
	return &Module{
		handler: &Handler{db: db, renderer: renderer, feed: feed, people: NewPGPeople(db)},
		feed:    feed,
	}
}

func (m *Module) Routes(r chi.Router, authn *auth.Authenticator) {
	r.Get("/calendar", m.handler.Index)

	// Public, token-authenticated: calendar apps cannot log in.
	// Must stay OUTSIDE any session/auth middleware.
	r.Get("/calendar/feed/{token}", m.feed.Serve)

	r.Group(func(r chi.Router) {
		r.Use(authn.RequireRoles(
			auth.RoleManager, auth.RoleEditor, auth.RoleViewer, auth.RoleScheduledViewer))
		r.Post("/calendar/blockouts", m.handler.AddBlockout)
		r.Get("/calendar/blockouts/{id}/edit", m.handler.EditBlockoutForm)
		r.Post("/calendar/blockouts/{id}/edit", m.handler.EditBlockout)
		r.Post("/calendar/blockouts/{id}/delete", m.handler.DeleteBlockout)

		r.Get("/calendar/feed", m.feed.Settings)
		r.Post("/calendar/feed/regenerate", m.feed.Regenerate)
	})
}
