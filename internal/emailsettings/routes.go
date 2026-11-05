package emailsettings

import (
	"embed"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/yourchurch/churchplanner/internal/platform/auth"
)

//go:embed templates/*.html
var templates embed.FS

func Templates() fs.FS {
	sub, err := fs.Sub(templates, "templates")
	if err != nil {
		panic(err)
	}
	return sub
}

func (h *Handler) Routes(r chi.Router, authn *auth.Authenticator) {
	r.Group(func(r chi.Router) {
		r.Use(authn.RequireRoles(auth.RoleAdmin))
		r.Route("/settings/email", func(r chi.Router) {
			r.Get("/", h.View)
			r.Post("/", h.Save)
			r.Post("/test", h.SendTest)
		})
	})
}
