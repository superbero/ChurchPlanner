package scheduling

import (
	"testing"
	"testing/fstest"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

func TestTemplatesParse(t *testing.T) {
	renderer, err := httpx.NewRenderer(fstest.MapFS{
		"layout.html": &fstest.MapFile{
			Data: []byte(`{{define "layout.html"}}{{template "content" .}}{{end}}`),
		},
	})
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}
	if err := renderer.Add("scheduling", Templates(), "*.html"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
}
