package communications

import (
	"embed"
	"io/fs"
	"net/http"

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

type Module struct{ renderer *httpx.Renderer }

func New(db *pgxpool.Pool, renderer *httpx.Renderer) *Module {
	_ = db // replace with repository wiring as you grow this module
	return &Module{renderer: renderer}
}

func (m *Module) Routes(r chi.Router) {
	r.Get("/communications", func(w http.ResponseWriter, req *http.Request) {
		m.renderer.Render(w, http.StatusOK, "communications_list", nil)
	})
}
