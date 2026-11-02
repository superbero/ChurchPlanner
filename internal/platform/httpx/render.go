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

type LanguageOption struct {
	Code  string
	Label string
}

var languageLabels = map[string]string{
	"en": "English",
	"fr": "Français",
	"es": "Español",
	"pt": "Português",
	"de": "Deutsch",
	"it": "Italiano",
	"nl": "Nederlands",
	"sw": "Kiswahili",
	"zh": "中文",
	"ko": "한국어",
}

var languageOrder = []string{"en", "fr", "es", "pt", "de", "it", "nl", "sw", "zh", "ko"}

func languageLabel(code string) string {
	if code == "" {
		return ""
	}
	if l, ok := languageLabels[code]; ok {
		return l
	}
	return strings.ToUpper(code)
}

func languageOptions() []LanguageOption {
	out := make([]LanguageOption, 0, len(languageOrder))
	for _, c := range languageOrder {
		out = append(out, LanguageOption{Code: c, Label: languageLabels[c]})
	}
	return out
}

// funcMap provides helper functions available in all templates.
// Registering them centrally means any template can use them without
// the file that declares it being aware.
func funcMap() template.FuncMap {
	return template.FuncMap{
		"contains":   strings.Contains,
		"hasPrefix":  strings.HasPrefix,
		"hasSuffix":  strings.HasSuffix,
		"split":      strings.Split,
		"splitAfter": strings.SplitAfter,
		"trimPrefix": strings.TrimPrefix,
		"trimSuffix": strings.TrimSuffix,
		"lower":         strings.ToLower,
		"upper":         strings.ToUpper,
		"languageLabel": languageLabel,
		"languageOptions": languageOptions,
	}
}

type Renderer struct {
	layout *template.Template
	pages  map[string]*template.Template
}

func NewRenderer(layoutFS fs.FS) (*Renderer, error) {
	// Funcs must be attached to the layout template BEFORE ParseFS, so that
	// every page parsed from a Clone of it inherits them.
	layout, err := template.New("layout.html").Funcs(funcMap()).ParseFS(layoutFS, "layout.html")
	if err != nil {
		return nil, fmt.Errorf("parse layout: %w", err)
	}
	return &Renderer{layout: layout, pages: make(map[string]*template.Template)}, nil
}

func (r *Renderer) Add(prefix string, fsys fs.FS, glob string) error {
	matches, err := fs.Glob(fsys, glob)
	if err != nil {
		return err
	}
	for _, m := range matches {
		t, err := r.layout.Clone() // inherits funcs from layout
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
