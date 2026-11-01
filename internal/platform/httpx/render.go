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
