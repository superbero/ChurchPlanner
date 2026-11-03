package songs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

func TestSongDetailHidesEditingControlsForAnonymousVisitors(t *testing.T) {
	renderer, err := httpx.NewRenderer(fstest.MapFS{
		"layout.html": &fstest.MapFile{
			Data: []byte(`{{define "layout.html"}}{{template "content" .}}{{end}}`),
		},
	})
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}
	if err := renderer.Add("songs", Templates(), "*.html"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/songs/song-1", nil)
	response := httptest.NewRecorder()
	renderer.Render(response, request, http.StatusOK, "songs_detail",
		map[string]any{
			"Song": Song{ID: "song-1", Title: "Test song"},
			"Arrangements": []struct {
				Arrangement
				Keys []ArrangementKey
			}{},
		})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	for _, forbidden := range []string{
		"/songs/song-1/edit",
		"/songs/song-1/arrangements/new",
		"/songs/song-1/practice-links",
		"/songs/song-1/history",
		"/songs/song-1/delete",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("anonymous song detail contains privileged link or form %q", forbidden)
		}
	}
}
