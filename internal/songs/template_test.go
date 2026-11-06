package songs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

func newSongTemplateRenderer(t *testing.T) *httpx.Renderer {
	t.Helper()
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
	return renderer
}

func TestNormalizeSongPageSize(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{input: 0, want: 10},
		{input: -1, want: 10},
		{input: 10, want: 10},
		{input: 15, want: 15},
		{input: 100, want: 15},
	}
	for _, tt := range tests {
		if got := normalizeSongPageSize(tt.input); got != tt.want {
			t.Errorf("normalizeSongPageSize(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestSongDetailHidesEditingControlsForAnonymousVisitors(t *testing.T) {
	renderer := newSongTemplateRenderer(t)
	response := httptest.NewRecorder()
	renderer.Render(response, httptest.NewRequest(http.MethodGet, "/songs/song-1", nil),
		http.StatusOK, "songs_detail", map[string]any{
			"Song": Song{ID: "song-1", Title: "Test song"},
			"Arrangements": []struct {
				Arrangement
				Keys []ArrangementKey
			}{},
		})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	for _, forbidden := range []string{
		"/songs/song-1/edit",
		"/songs/song-1/arrangements/new",
		"/songs/song-1/practice-links",
		"/songs/song-1/history",
		"/songs/song-1/delete",
	} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Errorf("anonymous song detail contains privileged link or form %q", forbidden)
		}
	}
}

func TestSongListRendersWithoutRemovalControlsWhenUserIsMissing(t *testing.T) {
	renderer := newSongTemplateRenderer(t)
	response := httptest.NewRecorder()
	renderer.Render(response, httptest.NewRequest(http.MethodGet, "/songs", nil),
		http.StatusOK, "songs_list", map[string]any{
			"Songs": []Song{
				{ID: "song-1", Title: "New song", RecentlyAddedUnused: true},
				{ID: "song-2", Title: "Unused song", NeverUsed: true},
			},
			"Filter":           Filter{},
			"Page":             SongPage{Total: 21, Page: 2, PageSize: 10},
			"PageCount":        3,
			"PreviousURL":      "/songs?page=1&page_size=10",
			"NextURL":          "/songs?page=3&page_size=10",
			"TagGroups":        []TagGroupWithTags{},
			"SelectedTagSlugs": map[string]bool{},
		})
	if response.Code != http.StatusOK {
		t.Fatalf("render status = %d, want %d", response.Code, http.StatusOK)
	}
	for _, control := range []string{"/songs/bulk-delete", `name="song_ids"`, "/songs/song-1/delete"} {
		if strings.Contains(response.Body.String(), control) {
			t.Errorf("song list without a user contains removal control %q", control)
		}
	}
	for _, expected := range []string{
		"New · not used", "Never used", "Page 2 / 3",
		`href="/songs?page=1&amp;page_size=10"`, `href="/songs?page=3&amp;page_size=10"`,
		`name="page_size"`, `value="10" selected`, `value="15"`, "10 per page", "Never used first",
		`id="song-library-search"`, `aria-controls="song-search-suggestions"`,
		`id="song-search-suggestions"`, `/songs/autocomplete?`,
	} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("song list is missing %q", expected)
		}
	}
}

func TestSongPreviewShowsDefaultArrangementAndEmptyState(t *testing.T) {
	renderer := newSongTemplateRenderer(t)
	render := func(data map[string]any) string {
		t.Helper()
		response := httptest.NewRecorder()
		renderer.RenderPartial(response, httptest.NewRequest(http.MethodGet, "/songs/song-1/preview", nil),
			http.StatusOK, "songs_preview", data)
		if response.Code != http.StatusOK {
			t.Fatalf("render status = %d, want %d", response.Code, http.StatusOK)
		}
		return response.Body.String()
	}

	chart := render(map[string]any{
		"Song": Song{ID: "song-1", Title: "Test tune", Author: "Test artist"},
		"Arrangement": Arrangement{
			Name: "Original", OriginalKey: "G", BPM: 92, Meter: "4/4",
		},
		"HasArrangement": true,
		"BaseKey":        "G",
		"DisplayKey":     "G",
		"Transpose":      0,
		"CanTranspose":   true,
		"Chart": []RenderedSection{{
			Label: "Verse",
			Lines: []ChartLine{{Chords: []ChordPos{{Position: 0, Chord: "G"}}, Lyric: "Amazing grace"}},
		}},
	})
	for _, expected := range []string{"Test tune", "Test artist", "Original", "Default arrangement", "G", "92 BPM", "Amazing grace", "Transpose chart", `aria-label="Choose chart key"`, `value="G" selected`, "Transpose up one semitone", "Reset"} {
		if !strings.Contains(chart, expected) {
			t.Errorf("preview is missing %q", expected)
		}
	}
	if strings.Contains(chart, `aria-label="Transpose up one semitone" disabled`) {
		t.Error("valid arrangement key has transposition disabled")
	}
	sharpKeyChart := render(map[string]any{
		"Song":           Song{ID: "song-1", Title: "Test tune"},
		"HasArrangement": true,
		"Arrangement":    Arrangement{Name: "Original"},
		"BaseKey":        "C",
		"DisplayKey":     "C#",
		"CanTranspose":   true,
		"Chart":          []RenderedSection{{Label: "Verse"}},
	})
	if !strings.Contains(sharpKeyChart, `/songs/song-1/pdf?key=C%23`) {
		t.Error("sharp key in the quick-view PDF URL is not safely encoded")
	}

	empty := render(map[string]any{
		"Song":           Song{ID: "song-2", Title: "No chart"},
		"HasArrangement": false,
	})
	if !strings.Contains(empty, "No arrangement yet") {
		t.Error("preview does not explain that the song has no arrangement")
	}
}
