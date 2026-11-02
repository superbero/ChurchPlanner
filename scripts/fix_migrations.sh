#!/usr/bin/env bash
# Rewrites internal/web/router.go with exactly-one-mount-per-module.
# Fixes: panic: chi: attempting to Mount() a handler on an existing path, '/plans'
set -euo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }
[ -f internal/web/router.go ] || { echo "✗ internal/web/router.go missing"; exit 1; }

echo "▸ [1/4] Current route registrations (for the record)"
grep -nE '\.Routes\(r\)|\.PublicRoutes\(r\)' internal/web/router.go || true

echo ""
echo "▸ [2/4] Rewriting router.go"
cat > internal/web/router.go <<'GOEOF'
package web

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/calendar"
	"github.com/yourchurch/churchplanner/internal/checkin"
	"github.com/yourchurch/churchplanner/internal/communications"
	"github.com/yourchurch/churchplanner/internal/people"
	"github.com/yourchurch/churchplanner/internal/platform/auth"
	"github.com/yourchurch/churchplanner/internal/platform/config"
	"github.com/yourchurch/churchplanner/internal/platform/httpx"
	"github.com/yourchurch/churchplanner/internal/scheduling"
	"github.com/yourchurch/churchplanner/internal/songs"
)

//go:embed static
var staticFS embed.FS

//go:embed templates
var templateFS embed.FS

type Deps struct {
	DB       *pgxpool.Pool
	Config   config.Config
	Sessions *scs.SessionManager
}

func NewRouter(d Deps) (http.Handler, error) {
	sub, err := fs.Sub(templateFS, "templates")
	if err != nil {
		return nil, err
	}

	renderer, err := httpx.NewRenderer(sub)
	if err != nil {
		return nil, err
	}
	if err := renderer.Add("web", sub, "login.html"); err != nil {
		return nil, err
	}
	// Every module appears exactly once here.
	for _, add := range []struct {
		prefix string
		fsys   fs.FS
	}{
		{"songs", songs.Templates()},
		{"scheduling", scheduling.Templates()},
		{"people", people.Templates()},
		{"calendar", calendar.Templates()},
		{"checkin", checkin.Templates()},
		{"communications", communications.Templates()},
	} {
		if err := renderer.Add(add.prefix, add.fsys, "*.html"); err != nil {
			return nil, err
		}
	}

	authn := &auth.Authenticator{DB: d.DB, Sessions: d.Sessions}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, err
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	// --- public: login ---
	r.Get("/login", func(w http.ResponseWriter, r *http.Request) {
		renderer.Render(w, http.StatusOK, "web_login", nil)
	})
	r.Post("/login", func(w http.ResponseWriter, r *http.Request) {
		email := r.FormValue("email")
		password := r.FormValue("password")

		var id, hash string
		err := d.DB.QueryRow(r.Context(),
			`SELECT id, password_hash FROM users WHERE email = $1`, email,
		).Scan(&id, &hash)
		if err != nil || !auth.CheckPassword(hash, password) {
			renderer.Render(w, http.StatusUnauthorized, "web_login", map[string]any{"Error": "Invalid credentials"})
			return
		}
		d.Sessions.Put(r.Context(), "user_id", id)
		http.Redirect(w, r, "/songs", http.StatusSeeOther)
	})
	r.Post("/logout", func(w http.ResponseWriter, r *http.Request) {
		d.Sessions.Destroy(r.Context())
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})

	// --- modules: construct once, mount once ---
	songsModule := songs.New(d.DB, renderer, songs.HandlerConfig{
		PublicBaseURL:   d.Config.PublicBaseURL,
		TranslateAPIURL: d.Config.TranslateAPIURL,
		TranslateAPIKey: d.Config.TranslateAPIKey,
		PrimaryLanguage: d.Config.PrimaryLanguage,
	})
	schedulingModule := scheduling.New(d.DB, renderer)

	// Practice links are mounted outside the auth group so volunteers
	// don't need an account.
	songsModule.PublicRoutes(r)

	// --- protected modules ---
	r.Group(func(r chi.Router) {
		r.Use(authn.RequireUser)

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/songs", http.StatusSeeOther)
		})

		songsModule.Routes(r)
		schedulingModule.Routes(r)
		people.New(d.DB, renderer).Routes(r)
		calendar.New(d.DB, renderer).Routes(r)
		checkin.New(d.DB, renderer).Routes(r)
		communications.New(d.DB, renderer).Routes(r)
	})

	return r, nil
}
GOEOF

echo ""
echo "▸ [3/4] Sanity check — each module must appear exactly once"
for m in songs.New scheduling.New 'people.New' 'calendar.New' 'checkin.New' 'communications.New'; do
  n=$(grep -c "$m" internal/web/router.go || true)
  echo "   $m: $n occurrence(s)"
  if [ "$n" -ne 1 ]; then
    echo "   ✗ unexpected count — inspect internal/web/router.go"
    exit 1
  fi
done

echo ""
echo "▸ [4/4] Build + restart"
go build ./... || { echo "✗ build failed — see errors above"; exit 1; }

docker compose -f deploy/docker-compose.yml up -d --build

echo ""
echo "▸ Tailing app logs for 20s (Ctrl-C to stop earlier)"
timeout 20 docker compose -f deploy/docker-compose.yml logs -f app 2>&1 || true

echo ""
echo "✅ Done."
echo ""
echo "Look for:  {\"level\":\"INFO\",\"msg\":\"listening\",\"addr\":\":8080\"}"
echo "Then open http://localhost:8080/plans"
