package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestRenderAcceptsTypedNilMap(t *testing.T) {
	files := fstest.MapFS{
		"layout.html": &fstest.MapFile{
			Data: []byte(`{{define "layout.html"}}{{template "content" .}}{{end}}`),
		},
		"page.html": &fstest.MapFile{
			Data: []byte(`{{define "content"}}Sign in{{end}}`),
		},
	}
	renderer, err := NewRenderer(files)
	if err != nil {
		t.Fatal(err)
	}
	if err := renderer.Add("test", files, "page.html"); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	var data map[string]any
	renderer.Render(response, httptest.NewRequest(http.MethodGet, "/login", nil), http.StatusOK, "test_page", data)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), "Sign in") {
		t.Errorf("body = %q, want login page content", response.Body.String())
	}
}
