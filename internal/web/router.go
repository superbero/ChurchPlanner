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
	"github.com/yourchurch/churchplanner/internal/reports"
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
	for _, add := range []struct {
		prefix string
		fsys   fs.FS
	}{
		{"songs", songs.Templates()},
		{"scheduling", scheduling.Templates()},
		{"people", people.Templates()},
		{"calendar", calendar.Templates()},
		{"reports", reports.Templates()},
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

	// --- Public: login ---
	r.Get("/login", func(w http.ResponseWriter, r *http.Request) {
		renderer.Render(w, r, http.StatusOK, "web_login", nil)
	})
	r.Post("/login", func(w http.ResponseWriter, r *http.Request) {
		email := r.FormValue("email")
		password := r.FormValue("password")

		var id, hash string
		err := d.DB.QueryRow(r.Context(),
			`SELECT id, password_hash FROM users WHERE email = $1`, email,
		).Scan(&id, &hash)
		if err != nil || !auth.CheckPassword(hash, password) {
			renderer.Render(w, r, http.StatusUnauthorized, "web_login", map[string]any{"Error": "Invalid credentials"})
			return
		}
		d.Sessions.Put(r.Context(), "user_id", id)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	r.Post("/logout", func(w http.ResponseWriter, r *http.Request) {
		d.Sessions.Destroy(r.Context())
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})

	// --- Modules: construct once, mount once ---
	songsModule := songs.New(d.DB, renderer, songs.HandlerConfig{
		PublicBaseURL:   d.Config.PublicBaseURL,
		TranslateAPIURL: d.Config.TranslateAPIURL,
		TranslateAPIKey: d.Config.TranslateAPIKey,
		PrimaryLanguage: d.Config.PrimaryLanguage,
	})
	schedulingModule := scheduling.New(d.DB, renderer)
	checkinModule := checkin.New(d.DB, renderer)

	// Public: no auth required
	songsModule.PublicRoutes(r)
	checkinModule.PublicRoutes(r)

	// teamLeaderOrAdmin: allows admin OR any user with a leader row in team_memberships.
	// Used only to gate the /plans subrouter — per-position writes are further
	// restricted inside the scheduling handlers via canManagePosition().
	teamLeaderOrAdmin := func(next http.Handler) http.Handler {
		return authn.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := auth.UserFrom(r.Context())
			if !ok {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			if u.Role == "admin" {
				next.ServeHTTP(w, r)
				return
			}
			var isLeader bool
			if err := d.DB.QueryRow(r.Context(),
				`SELECT EXISTS(SELECT 1 FROM team_memberships WHERE user_id = $1 AND role = 'leader')`,
				u.ID).Scan(&isLeader); err != nil {
				httpx.ServerError(w, err)
				return
			}
			if isLeader {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "forbidden", http.StatusForbidden)
		}))
	}

	// --- Authenticated area ---
	r.Group(func(r chi.Router) {
		r.Use(authn.RequireUser)

		// Dashboard
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			u, _ := auth.UserFrom(r.Context())
			if u.Role == "admin" {
				http.Redirect(w, r, "/plans", http.StatusSeeOther)
				return
			}
			var isLeader bool
			if err := d.DB.QueryRow(r.Context(),
				`SELECT EXISTS(SELECT 1 FROM team_memberships WHERE user_id = $1 AND role = 'leader')`,
				u.ID).Scan(&isLeader); err != nil {
				httpx.ServerError(w, err)
				return
			}
			if isLeader {
				http.Redirect(w, r, "/plans", http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, "/my-schedule", http.StatusSeeOther)
		})

		// Shared across all logged-in users
		schedulingModule.MyScheduleRoutes(r)
		calendar.New(d.DB, renderer).Routes(r, authn)
		checkinModule.StaffRoutes(r, authn)
		songsModule.Routes(r, authn)

		// /plans: admins + team leaders (per-position writes further scoped)
		r.Group(func(r chi.Router) {
			r.Use(teamLeaderOrAdmin)
			schedulingModule.Routes(r, authn)
		})

		// Admin only
		r.Group(func(r chi.Router) {
			r.Use(authn.RequireRoles("admin"))
			people.New(d.DB, renderer).Routes(r)
			reports.New(d.DB, renderer).Routes(r)
		})
	})

	return r, nil
}
