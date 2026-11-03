package songs

import (
	"embed"
	"io/fs"

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

type Module struct{ handler *Handler }

func New(db *pgxpool.Pool, renderer *httpx.Renderer, cfg HandlerConfig) *Module {
	arrRepo := NewArrangementRepository(db)
	health := NewHealthService(db)
	translator := NewTranslator(cfg.TranslateAPIURL, cfg.TranslateAPIKey)
	return &Module{handler: &Handler{
		svc:      NewService(NewRepository(db), NewHistoryRepository(db)),
		arr:      NewArrangementService(arrRepo),
		tags:     NewTagService(db),
		revs:     NewRevisionRepository(db),
		practice: NewPracticeRepository(db),
		health:   health,
		sugg:     NewSuggestionService(db, health),
		trans:    translator,
		cfg:      cfg,
		renderer: renderer,
	}}
}

// PublicRoutes are mounted outside auth middleware (practice links).
func (m *Module) PublicRoutes(r chi.Router) {
	r.Get("/practice/{token}", m.handler.ViewPractice)
	r.Get("/practice/{token}/pdf", m.handler.ViewPracticePDF)
}

// Routes registers all song paths. Read-only paths are available to everyone.
// Modify paths are gated behind an Admin role check.
func (m *Module) Routes(r chi.Router, authn *auth.Authenticator) {
	r.Route("/songs", func(r chi.Router) {

		// --- Read-Only (All Users) ---
		r.Get("/", m.handler.List)
		r.Get("/search", m.handler.SearchArrangementsJSON)
		r.Get("/suggest", m.handler.Suggestions)
		r.Get("/{id}", m.handler.View)
		r.Get("/{id}/pdf", m.handler.DownloadPDF)
		r.Get("/{id}/arrangements/{aid}", m.handler.ViewArrangement)
		r.Get("/{id}/arrangements/{aid}/pdf", m.handler.DownloadPDF)

		// --- Admin Only (Modifying Data) ---
		r.Group(func(r chi.Router) {
			r.Use(authn.RequireRoles("admin", "editor"))

			r.Get("/new", m.handler.New)
			r.Post("/", m.handler.Create)
			r.Get("/import", m.handler.ShowImport)
			r.Post("/import", m.handler.Import)

			// Health dashboard is an admin insight tool
			r.Get("/reports/health", m.handler.Health)

			// Tag management
			r.Get("/tags", m.handler.TagsIndex)
			r.Post("/tags/groups", m.handler.CreateTagGroup)
			r.Post("/tags/tags", m.handler.CreateTag)
			r.Post("/tags/{id}/delete", m.handler.DeleteTag)

			// Song editing
			r.Get("/{id}/edit", m.handler.Edit)
			r.Post("/{id}", m.handler.Update)
			r.Post("/{id}/delete", m.handler.Delete)
			r.Post("/{id}/history", m.handler.AddHistory)
			r.Post("/{id}/practice-links", m.handler.CreatePracticeLink)

			// Arrangements
			r.Get("/{id}/arrangements/new", m.handler.NewArrangement)
			r.Post("/{id}/arrangements", m.handler.CreateArrangement)
			r.Get("/{id}/arrangements/{aid}/edit", m.handler.EditArrangement)
			r.Post("/{id}/arrangements/{aid}", m.handler.UpdateArrangement)
			r.Post("/{id}/arrangements/{aid}/delete", m.handler.DeleteArrangement)
			r.Post("/{id}/arrangements/{aid}/set-default", m.handler.SetDefaultArrangement)
			r.Get("/{id}/arrangements/{aid}/history", m.handler.ArrangementHistory)
			r.Post("/{id}/arrangements/{aid}/history/{rid}/restore", m.handler.RestoreRevision)

			// Keys
			r.Post("/{id}/arrangements/{aid}/keys", m.handler.CreateArrangementKey)
			r.Post("/{id}/arrangements/{aid}/keys/{kid}/delete", m.handler.DeleteArrangementKey)
			r.Post("/{id}/arrangements/{aid}/keys/{kid}/set-default", m.handler.SetDefaultArrangementKey)

			// Translations
			r.Get("/{id}/arrangements/{aid}/translations/new", m.handler.NewTranslationForm)
			r.Post("/{id}/arrangements/{aid}/translations", m.handler.SaveTranslation)
			r.Post("/{id}/arrangements/{aid}/translations/{tid}/delete", m.handler.DeleteTranslation)
			r.Post("/{id}/arrangements/{aid}/translate", m.handler.TranslateSource)
		})
	})
}
