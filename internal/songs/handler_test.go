package songs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
)

func TestParseArrangementInputPreservesPrimaryLanguage(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/songs/song-1/arrangements",
		strings.NewReader("name=Acoustic&primary_language=fr&source_content=bonjour"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	input := parseArrangementInput(request)
	if input.PrimaryLanguage != "fr" {
		t.Errorf("PrimaryLanguage = %q, want %q", input.PrimaryLanguage, "fr")
	}
	if input.Format != "chordpro" {
		t.Errorf("Format = %q, want default %q", input.Format, "chordpro")
	}
}

func TestRoutesRegisterSuggestions(t *testing.T) {
	router := chi.NewRouter()
	module := &Module{handler: &Handler{}}
	module.Routes(router, &auth.Authenticator{})

	foundSuggestions := false
	foundAutocomplete := false
	err := chi.Walk(router, func(method, route string, _ http.Handler,
		_ ...func(http.Handler) http.Handler) error {
		if method == http.MethodGet && route == "/songs/suggest" {
			foundSuggestions = true
		}
		if method == http.MethodGet && route == "/songs/autocomplete" {
			foundAutocomplete = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}
	if !foundSuggestions {
		t.Fatal("GET /songs/suggest is not registered")
	}
	if !foundAutocomplete {
		t.Fatal("GET /songs/autocomplete is not registered")
	}
}

func TestIsSuggestionQueryRequiresThreeCharacters(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{query: "ab", want: false},
		{query: " abc ", want: true},
		{query: "éé", want: false},
		{query: "ééa", want: true},
	}
	for _, test := range tests {
		if got := isSuggestionQuery(test.query); got != test.want {
			t.Errorf("isSuggestionQuery(%q) = %t, want %t", test.query, got, test.want)
		}
	}
}
