#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-churchplanner}"
mkdir -p "$ROOT" && cd "$ROOT"

echo "▸ Creating directory tree..."
mkdir -p cmd/server
mkdir -p internal/platform/{config,database,auth,httpx,validate}
mkdir -p internal/web/{templates,static/css,static/js}
mkdir -p internal/songs/templates
mkdir -p migrations deploy scripts

# ───────────────────────── go.mod ─────────────────────────
cat > go.mod <<'GOEOF'
module github.com/yourchurch/churchplanner

go 1.23
GOEOF

# ───────────────────────── .gitignore ─────────────────────────
cat > .gitignore <<'EOF'
/bin/
.env
deploy/.env
*.log
.DS_Store
EOF

# ───────────────────────── cmd/server/main.go ─────────────────────────
cat > cmd/server/main.go <<'GOEOF'
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
	"github.com/yourchurch/churchplanner/internal/platform/config"
	"github.com/yourchurch/churchplanner/internal/platform/database"
	"github.com/yourchurch/churchplanner/internal/web"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connect", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := database.Migrate(ctx, db); err != nil {
		logger.Error("database migrate", "err", err)
		os.Exit(1)
	}

	sessions := auth.NewSessionManager(db)

	handler, err := web.NewRouter(web.Deps{
		DB:       db,
		Config:   cfg,
		Sessions: sessions,
	})
	if err != nil {
		logger.Error("router", "err", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           sessions.LoadAndSave(handler),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("listening", "addr", srv.Addr, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("serve", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	logger.Info("stopped")
}
GOEOF

# ───────────────────────── platform/config ─────────────────────────
cat > internal/platform/config/config.go <<'GOEOF'
package config

import (
	"errors"
	"os"
)

type Config struct {
	Port        string
	DatabaseURL string
	Env         string
}

func Load() (Config, error) {
	c := Config{
		Port:        getenv("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Env:         getenv("APP_ENV", "development"),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	return c, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
GOEOF

# ───────────────────────── platform/database ─────────────────────────
cat > internal/platform/database/database.go <<'GOEOF'
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/yourchurch/churchplanner/migrations"
)

func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse db url: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MaxConnLifetime = time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return pool, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	return goose.UpContext(ctx, db, ".")
}
GOEOF

# ───────────────────────── platform/auth ─────────────────────────
cat > internal/platform/auth/session.go <<'GOEOF'
package auth

import (
	"net/http"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewSessionManager(db *pgxpool.Pool) *scs.SessionManager {
	sm := scs.New()
	sm.Store = pgxstore.New(db)
	sm.Lifetime = 7 * 24 * time.Hour
	sm.Cookie.Name = "churchplanner_session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Path = "/"
	return sm
}
GOEOF

cat > internal/platform/auth/password.go <<'GOEOF'
package auth

import "golang.org/x/crypto/bcrypt"

func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}
GOEOF

cat > internal/platform/auth/middleware.go <<'GOEOF'
package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ctxKey int

const userKey ctxKey = iota

type User struct {
	ID    string
	Name  string
	Email string
	Role  string
}

type Authenticator struct {
	DB       *pgxpool.Pool
	Sessions *scs.SessionManager
}

func (a *Authenticator) CurrentUser(r *http.Request) (User, bool) {
	id := a.Sessions.GetString(r.Context(), "user_id")
	if id == "" {
		return User{}, false
	}
	var u User
	err := a.DB.QueryRow(r.Context(),
		`SELECT id, name, email, role FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Name, &u.Email, &u.Role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			a.Sessions.Remove(r.Context(), "user_id")
		}
		return User{}, false
	}
	return u, true
}

func (a *Authenticator) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := a.CurrentUser(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func (a *Authenticator) RequireRole(role string, next http.Handler) http.Handler {
	return a.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := UserFrom(r.Context())
		if u.Role != role && u.Role != "admin" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey).(User)
	return u, ok
}
GOEOF

# ───────────────────────── platform/httpx ─────────────────────────
cat > internal/platform/httpx/render.go <<'GOEOF'
package httpx

import (
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
)

type Renderer struct {
	layout *template.Template
	pages  map[string]*template.Template
}

func NewRenderer(layoutFS fs.FS) (*Renderer, error) {
	layout, err := template.ParseFS(layoutFS, "layout.html")
	if err != nil {
		return nil, fmt.Errorf("parse layout: %w", err)
	}
	return &Renderer{layout: layout, pages: make(map[string]*template.Template)}, nil
}

// Add parses every file matching glob from fsys as a page.
// The rendered name is "<prefix>_<filename>" so modules can't collide.
func (r *Renderer) Add(prefix string, fsys fs.FS, glob string) error {
	matches, err := fs.Glob(fsys, glob)
	if err != nil {
		return err
	}
	for _, m := range matches {
		t, err := r.layout.Clone()
		if err != nil {
			return err
		}
		if _, err := t.ParseFS(fsys, m); err != nil {
			return fmt.Errorf("parse %s: %w", m, err)
		}
		name := prefix + "_" + strings.TrimSuffix(path.Base(m), ".html")
		r.pages[name] = t
	}
	return nil
}

func (r *Renderer) Render(w http.ResponseWriter, status int, page string, data any) {
	t, ok := r.pages[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.ExecuteTemplate(w, "layout.html", data); err != nil {
		slog.Error("render", "page", page, "err", err)
	}
}

func ServerError(w http.ResponseWriter, err error) {
	slog.Error("handler error", "err", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
GOEOF

# ───────────────────────── web/router.go ─────────────────────────
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
	for _, add := range []struct {
		prefix string
		fsys   fs.FS
	}{
		{"songs", songs.Templates()},
		{"people", people.Templates()},
		{"scheduling", scheduling.Templates()},
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

	// --- protected modules ---
	r.Group(func(r chi.Router) {
		r.Use(authn.RequireUser)

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/songs", http.StatusSeeOther)
		})

		songs.New(d.DB, renderer).Routes(r)
		people.New(d.DB, renderer).Routes(r)
		scheduling.New(d.DB, renderer).Routes(r)
		calendar.New(d.DB, renderer).Routes(r)
		checkin.New(d.DB, renderer).Routes(r)
		communications.New(d.DB, renderer).Routes(r)
	})

	return r, nil
}
GOEOF

# ───────────────────────── web templates + static ─────────────────────────
cat > internal/web/templates/layout.html <<'HTMLEOF'
{{define "layout.html"}}
<!doctype html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>{{block "title" .}}Church Planner{{end}}</title>
    <link rel="stylesheet" href="/static/css/app.css">
    <script src="/static/js/app.js" defer></script>
</head>
<body>
    <header class="nav">
        <a href="/" class="brand">Church Planner</a>
        <nav>
            <a href="/songs">Songs</a>
            <a href="/people">People</a>
            <a href="/scheduling">Scheduling</a>
            <a href="/calendar">Calendar</a>
        </nav>
        <form method="post" action="/logout" class="logout"><button>Sign out</button></form>
    </header>
    <main>
        {{template "content" .}}
    </main>
</body>
</html>
{{end}}
HTMLEOF

cat > internal/web/templates/login.html <<'HTMLEOF'
{{define "content"}}
<h1>Sign in</h1>
{{with .Error}}<p class="error">{{.}}</p>{{end}}
<form method="post" action="/login">
    <label>Email <input type="email" name="email" required></label>
    <label>Password <input type="password" name="password" required></label>
    <button type="submit">Sign in</button>
</form>
{{end}}
HTMLEOF

cat > internal/web/static/css/app.css <<'CSSEOF'
:root { --fg:#1f2933; --bg:#f7f9fc; --accent:#2f6feb; --border:#d9e2ec; }
* { box-sizing: border-box; }
body { margin:0; font-family: system-ui, -apple-system, "Segoe UI", Roboto, sans-serif; color:var(--fg); background:var(--bg); }
.nav { display:flex; align-items:center; gap:1rem; padding:.75rem 1.25rem; background:#fff; border-bottom:1px solid var(--border); }
.nav .brand { font-weight:700; margin-right:1rem; color:var(--accent); text-decoration:none; }
.nav nav a { margin-right:1rem; color:var(--fg); text-decoration:none; }
.nav .logout { margin-left:auto; }
main { max-width:960px; margin:2rem auto; padding:0 1rem; }
table { width:100%; border-collapse:collapse; background:#fff; }
th, td { text-align:left; padding:.6rem .75rem; border-bottom:1px solid var(--border); }
label { display:block; margin:.75rem 0; }
input, textarea { width:100%; padding:.5rem; border:1px solid var(--border); border-radius:6px; font:inherit; }
button, .button { display:inline-block; padding:.55rem .9rem; background:var(--accent); color:#fff; border:0; border-radius:6px; cursor:pointer; text-decoration:none; }
.error { color:#b42318; }
CSSEOF

cat > internal/web/static/js/app.js <<'JSEOF'
// htmx or progressive enhancement hooks go here.
console.debug("churchplanner ui ready");
JSEOF

# ───────────────────────── songs module (fully worked example) ─────────────────────────
cat > internal/songs/model.go <<'GOEOF'
package songs

import "time"

type Song struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	Key       string    `json:"key"`
	Tempo     int       `json:"tempo"`
	Lyrics    string    `json:"lyrics"`
	Chords    string    `json:"chords"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateSongInput struct {
	Title  string
	Author string
	Key    string
	Tempo  int
	Lyrics string
	Chords string
}
GOEOF

cat > internal/songs/repository.go <<'GOEOF'
package songs

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("song not found")

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const columns = `id, title, author, key, tempo, lyrics, chords, created_at, updated_at`

func scan(row pgx.Row) (Song, error) {
	var s Song
	err := row.Scan(&s.ID, &s.Title, &s.Author, &s.Key, &s.Tempo, &s.Lyrics, &s.Chords, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (r *Repository) List(ctx context.Context) ([]Song, error) {
	rows, err := r.db.Query(ctx, `SELECT `+columns+` FROM songs ORDER BY title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Song
	for rows.Next() {
		s, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id string) (Song, error) {
	s, err := scan(r.db.QueryRow(ctx, `SELECT `+columns+` FROM songs WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Song{}, ErrNotFound
	}
	return s, err
}

func (r *Repository) Create(ctx context.Context, in CreateSongInput) (Song, error) {
	return scan(r.db.QueryRow(ctx, `
		INSERT INTO songs (title, author, key, tempo, lyrics, chords)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+columns,
		in.Title, in.Author, in.Key, in.Tempo, in.Lyrics, in.Chords))
}

func (r *Repository) Update(ctx context.Context, id string, in CreateSongInput) (Song, error) {
	s, err := scan(r.db.QueryRow(ctx, `
		UPDATE songs
		SET title=$2, author=$3, key=$4, tempo=$5, lyrics=$6, chords=$7, updated_at=now()
		WHERE id=$1
		RETURNING `+columns,
		id, in.Title, in.Author, in.Key, in.Tempo, in.Lyrics, in.Chords))
	if errors.Is(err, pgx.ErrNoRows) {
		return Song{}, ErrNotFound
	}
	return s, err
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM songs WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
GOEOF

cat > internal/songs/service.go <<'GOEOF'
package songs

import (
	"context"
	"errors"
	"strings"
)

var ErrInvalidInput = errors.New("title is required")

type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

func (s *Service) List(ctx context.Context) ([]Song, error) { return s.repo.List(ctx) }

func (s *Service) Get(ctx context.Context, id string) (Song, error) {
	if id == "" {
		return Song{}, ErrNotFound
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, in CreateSongInput) (Song, error) {
	if err := validate(in); err != nil {
		return Song{}, err
	}
	return s.repo.Create(ctx, in)
}

func (s *Service) Update(ctx context.Context, id string, in CreateSongInput) (Song, error) {
	if err := validate(in); err != nil {
		return Song{}, err
	}
	return s.repo.Update(ctx, id, in)
}

func (s *Service) Delete(ctx context.Context, id string) error { return s.repo.Delete(ctx, id) }

func validate(in CreateSongInput) error {
	if strings.TrimSpace(in.Title) == "" {
		return ErrInvalidInput
	}
	return nil
}
GOEOF

cat > internal/songs/handler.go <<'GOEOF'
package songs

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

type Handler struct {
	svc      *Service
	renderer *httpx.Renderer
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	h.renderer.Render(w, http.StatusOK, "songs_list", map[string]any{"Songs": items})
}

func (h *Handler) New(w http.ResponseWriter, r *http.Request) {
	h.renderer.Render(w, http.StatusOK, "songs_form", map[string]any{"Song": Song{}})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	in := parseInput(r)
	if _, err := h.svc.Create(r.Context(), in); err != nil {
		h.renderer.Render(w, http.StatusUnprocessableEntity, "songs_form", map[string]any{
			"Song":  Song{Title: in.Title, Author: in.Author, Key: in.Key, Lyrics: in.Lyrics, Chords: in.Chords},
			"Error": err.Error(),
		})
		return
	}
	http.Redirect(w, r, "/songs", http.StatusSeeOther)
}

func (h *Handler) Edit(w http.ResponseWriter, r *http.Request) {
	song, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderer.Render(w, http.StatusOK, "songs_form", map[string]any{"Song": song})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if _, err := h.svc.Update(r.Context(), chi.URLParam(r, "id"), parseInput(r)); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/songs", http.StatusSeeOther)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/songs", http.StatusSeeOther)
}

func parseInput(r *http.Request) CreateSongInput {
	tempo, _ := strconv.Atoi(r.FormValue("tempo"))
	return CreateSongInput{
		Title:  strings.TrimSpace(r.FormValue("title")),
		Author: strings.TrimSpace(r.FormValue("author")),
		Key:    strings.TrimSpace(r.FormValue("key")),
		Tempo:  tempo,
		Lyrics: r.FormValue("lyrics"),
		Chords: r.FormValue("chords"),
	}
}
GOEOF

cat > internal/songs/routes.go <<'GOEOF'
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
GOEOF

cat > internal/songs/templates/list.html <<'HTMLEOF'
{{define "content"}}
<h1>Songs</h1>
<p><a class="button" href="/songs/new">Add song</a></p>
<table>
    <thead><tr><th>Title</th><th>Author</th><th>Key</th><th></th></tr></thead>
    <tbody>
    {{range .Songs}}
        <tr>
            <td>{{.Title}}</td>
            <td>{{.Author}}</td>
            <td>{{.Key}}</td>
            <td><a href="/songs/{{.ID}}/edit">Edit</a></td>
        </tr>
    {{else}}
        <tr><td colspan="4">No songs yet.</td></tr>
    {{end}}
    </tbody>
</table>
{{end}}
HTMLEOF

cat > internal/songs/templates/form.html <<'HTMLEOF'
{{define "content"}}
<h1>{{if .Song.ID}}Edit song{{else}}New song{{end}}</h1>
{{with .Error}}<p class="error">{{.}}</p>{{end}}
<form method="post" action="{{if .Song.ID}}/songs/{{.Song.ID}}{{else}}/songs{{end}}">
    <label>Title <input name="title" value="{{.Song.Title}}" required></label>
    <label>Author <input name="author" value="{{.Song.Author}}"></label>
    <label>Key <input name="key" value="{{.Song.Key}}"></label>
    <label>Tempo <input name="tempo" type="number" value="{{.Song.Tempo}}"></label>
    <label>Lyrics <textarea name="lyrics" rows="6">{{.Song.Lyrics}}</textarea></label>
    <label>Chords <textarea name="chords" rows="6">{{.Song.Chords}}</textarea></label>
    <button type="submit">Save</button>
</form>
{{end}}
HTMLEOF

# ───────────────────────── stub modules ─────────────────────────
write_stub() {
  mod="$1"
  mkdir -p "internal/$mod/templates"
  cat > "internal/$mod/routes.go" <<GOEOF
package $mod

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
	r.Get("/$mod", func(w http.ResponseWriter, req *http.Request) {
		m.renderer.Render(w, http.StatusOK, "${mod}_list", nil)
	})
}
GOEOF

  cat > "internal/$mod/templates/list.html" <<HTMLEOF
{{define "content"}}
<h1>$mod</h1>
<p>Module scaffolded. Implement model / repository / service / handler here.</p>
{{end}}
HTMLEOF
}

for m in people scheduling calendar checkin communications; do
  write_stub "$m"
done

# ───────────────────────── migrations ─────────────────────────
cat > migrations/embed.go <<'GOEOF'
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
GOEOF

cat > migrations/00001_init.sql <<'SQLEOF'
-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'volunteer',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    token  TEXT PRIMARY KEY,
    data   BYTEA NOT NULL,
    expiry TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions (expiry);

CREATE TABLE songs (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title      TEXT NOT NULL,
    author     TEXT NOT NULL DEFAULT '',
    key        TEXT NOT NULL DEFAULT '',
    tempo      INTEGER NOT NULL DEFAULT 0,
    lyrics     TEXT NOT NULL DEFAULT '',
    chords     TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE service_plans (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title      TEXT NOT NULL,
    service_at TIMESTAMPTZ NOT NULL,
    notes      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE plan_items (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id          UUID NOT NULL REFERENCES service_plans(id) ON DELETE CASCADE,
    position         INTEGER NOT NULL,
    kind             TEXT NOT NULL,
    title            TEXT NOT NULL,
    song_id          UUID REFERENCES songs(id) ON DELETE SET NULL,
    duration_minutes INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE assignments (
    id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id UUID NOT NULL REFERENCES service_plans(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role    TEXT NOT NULL,
    status  TEXT NOT NULL DEFAULT 'pending',
    UNIQUE (plan_id, user_id, role)
);

CREATE TABLE events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title      TEXT NOT NULL,
    starts_at  TIMESTAMPTZ NOT NULL,
    ends_at    TIMESTAMPTZ NOT NULL,
    location   TEXT NOT NULL DEFAULT '',
    ministry   TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed an admin so you can log in immediately. CHANGE THE PASSWORD.
INSERT INTO users (name, email, password_hash, role)
VALUES ('Admin', 'admin@example.com', crypt('changeme', gen_salt('bf')), 'admin')
ON CONFLICT (email) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS assignments;
DROP TABLE IF EXISTS plan_items;
DROP TABLE IF EXISTS service_plans;
DROP TABLE IF EXISTS songs;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
SQLEOF

# ───────────────────────── deploy ─────────────────────────
cat > deploy/Dockerfile <<'DOCKEREOF'
# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS build
WORKDIR /src
RUN apk add --no-cache git

COPY go.mod go.sum* ./
RUN go mod download || true

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 app
USER app
WORKDIR /app
COPY --from=build /out/server /app/server
EXPOSE 8080
ENTRYPOINT ["/app/server"]
DOCKEREOF

cat > deploy/docker-compose.yml <<'YAMLEOF'
services:
  app:
    build:
      context: ..
      dockerfile: deploy/Dockerfile
    env_file: .env
    ports:
      - "${APP_PORT:-8080}:8080"
    depends_on:
      db:
        condition: service_healthy
    restart: unless-stopped

  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: ${POSTGRES_USER:-church}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-church}
      POSTGRES_DB: ${POSTGRES_DB:-churchplanner}
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER:-church} -d ${POSTGRES_DB:-churchplanner}"]
      interval: 5s
      timeout: 5s
      retries: 10
    restart: unless-stopped

volumes:
  pgdata:
YAMLEOF

cat > deploy/.env.example <<'ENVEOF'
APP_ENV=development
PORT=8080
APP_PORT=8080
DATABASE_URL=postgres://church:change-me@db:5432/churchplanner?sslmode=disable
POSTGRES_USER=church
POSTGRES_PASSWORD=change-me
POSTGRES_DB=churchplanner
ENVEOF

# ───────────────────────── scripts/new-module.sh ─────────────────────────
cat > scripts/new-module.sh <<'SCRIPTEOF'
#!/usr/bin/env bash
# Usage: ./scripts/new-module.sh events
set -euo pipefail
mod="${1:?module name required}"
mkdir -p "internal/$mod/templates"
echo "Created internal/$mod/. Next steps:"
echo "  1. Add model.go, repository.go, service.go, handler.go"
echo "  2. Copy routes.go from internal/songs/routes.go and adapt"
echo "  3. Add template partials under internal/$mod/templates/*.html"
echo "  4. Register in internal/web/router.go (renderer.Add + New(...).Routes(r))"
SCRIPTEOF
chmod +x scripts/new-module.sh

# ───────────────────────── Makefile ─────────────────────────
cat > Makefile <<'MAKEEOF'
.PHONY: tidy run build test docker-up docker-down

tidy:
	go mod tidy

run:
	go run ./cmd/server

build:
	CGO_ENABLED=0 go build -o bin/server ./cmd/server

test:
	go test ./...

docker-up:
	cd deploy && cp -n .env.example .env && docker compose up --build

docker-down:
	cd deploy && docker compose down
MAKEEOF

# ───────────────────────── README ─────────────────────────
cat > README.md <<'MDEOF'
# Church Planner

Modular monolith in Go for church service planning: songs, people, scheduling, calendar.

## First run (local)

    go mod tidy           # resolves dependencies and writes go.sum
    cd deploy
    cp .env.example .env
    cd ..
    docker compose -f deploy/docker-compose.yml up --build

Open http://localhost:8080 — sign in with `admin@example.com` / `changeme`.
**Change that password immediately** (edit `migrations/00001_init.sql` before first run, or
delete the seeded row and create your own).

## Module rules (enforced by `internal/`)

- A module may import `internal/platform/*` and its own subpackages only.
- A module must NOT import another module's packages. If scheduling needs song data,
  add a small interface in `internal/platform` or an explicit read model.
- Each module owns its tables. No cross-module joins in SQL.
- Each module exposes `New(db, renderer) *Module` and `(m) Routes(chi.Router)`.
- Templates live under `internal/<module>/templates/` and define `{{define "content"}}`.

## Adding a module

    ./scripts/new-module.sh events

Then wire it into `internal/web/router.go`:

    renderer.Add("events", events.Templates(), "*.html")
    ...
    events.New(d.DB, renderer).Routes(r)

## Safe update procedure

1. Write migration as **additive** (add column / table, never rename/drop in the same deploy).
2. Add module code behind new routes; keep old routes working.
3. `go test ./...`.
4. `docker compose up --build -d`.
5. Once stable, ship a second migration that removes the old column.
MDEOF

echo ""
echo "✅ Scaffold created in $(pwd)"
echo ""
echo "Next:"
echo "  cd $ROOT"
echo "  go mod tidy"
echo "  docker compose -f deploy/docker-compose.yml up --build"
echo "  → http://localhost:8080  (admin@example.com / changeme)"
