package httpx

import (
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
)


func isEditorOrAdmin(role string) bool {
	return role == "admin" || role == "editor"
}

func roleLabel(role string) string {
	switch role {
	case "admin":
		return "Organization Administrator"
	case "manager":
		return "Manager"
	case "editor":
		return "Editor"
	case "viewer":
		return "Viewer"
	case "scheduled_viewer":
		return "Scheduled Viewer"
	default:
		return "Unknown role"
	}
}

func funcMap() template.FuncMap {
	return template.FuncMap{
		"contains":   strings.Contains,
		"hasPrefix":  strings.HasPrefix,
		"hasSuffix":  strings.HasSuffix,
		"split":      strings.Split,
		"splitAfter": strings.SplitAfter,
		"trimPrefix": strings.TrimPrefix,
		"trimSuffix": strings.TrimSuffix,
		"lower":      strings.ToLower,
		"upper":      strings.ToUpper,
		"languageLabel":   languageLabel,
		"isEditorOrAdmin": isEditorOrAdmin,
		"roleLabel":       roleLabel,
		"languageOptions": languageOptions,
	}
}

type LanguageOption struct {
	Code  string
	Label string
}

var languageLabels = map[string]string{
	"en": "English", "fr": "Français", "es": "Español", "pt": "Português",
	"de": "Deutsch", "it": "Italiano", "nl": "Nederlands", "sw": "Kiswahili",
	"zh": "中文", "ko": "한국어",
}
var languageOrder = []string{"en", "fr", "es", "pt", "de", "it", "nl", "sw", "zh", "ko"}

func languageLabel(code string) string {
	if code == "" { return "" }
	if l, ok := languageLabels[code]; ok { return l }
	return strings.ToUpper(code)
}

func languageOptions() []LanguageOption {
	out := make([]LanguageOption, 0, len(languageOrder))
	for _, c := range languageOrder {
		out = append(out, LanguageOption{Code: c, Label: languageLabels[c]})
	}
	return out
}

type Renderer struct {
	layout *template.Template
	pages  map[string]*template.Template
}

func NewRenderer(layoutFS fs.FS) (*Renderer, error) {
	layout, err := template.New("layout.html").Funcs(funcMap()).ParseFS(layoutFS, "layout.html")
	if err != nil {
		return nil, fmt.Errorf("parse layout: %w", err)
	}
	return &Renderer{layout: layout, pages: make(map[string]*template.Template)}, nil
}

func (r *Renderer) Add(prefix string, fsys fs.FS, glob string) error {
	matches, err := fs.Glob(fsys, glob)
	if err != nil { return err }
	for _, m := range matches {
		t, err := r.layout.Clone()
		if err != nil { return err }
		if _, err := t.ParseFS(fsys, m); err != nil {
			return fmt.Errorf("parse %s: %w", m, err)
		}
		name := prefix + "_" + strings.TrimSuffix(path.Base(m), ".html")
		r.pages[name] = t
	}
	return nil
}

// Render now requires the request so it can inject CurrentUser into the template.
func (r *Renderer) Render(w http.ResponseWriter, req *http.Request, status int, page string, data any) {
	t, ok := r.pages[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}

	// Ensure data is a map so we can inject the user
	var dataMap map[string]any
	if data == nil {
		dataMap = make(map[string]any)
	} else if m, ok := data.(map[string]any); ok {
		dataMap = m
	} else {
		http.Error(w, "renderer requires map[string]any", http.StatusInternalServerError)
		return
	}

	if u, ok := auth.UserFrom(req.Context()); ok {
		dataMap["CurrentUser"] = u
	} else {
		dataMap["CurrentUser"] = nil
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.ExecuteTemplate(w, "layout.html", dataMap); err != nil {
		slog.Error("render", "page", page, "err", err)
	}
}

func ServerError(w http.ResponseWriter, err error) {
	slog.Error("handler error", "err", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
