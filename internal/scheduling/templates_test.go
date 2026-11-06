package scheduling

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

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

func TestNewPlanFormUsesLatestServiceWindowForType(t *testing.T) {
	renderer, err := httpx.NewRenderer(fstest.MapFS{
		"layout.html": &fstest.MapFile{
			Data: []byte(`{{define "layout.html"}}{{template "content" .}}{{end}}`),
		},
	})
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}
	if err := renderer.Add("scheduling", Templates(), "plan_form.html"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	start := time.Date(2026, time.November, 8, 9, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	response := httptest.NewRecorder()
	renderer.Render(response, httptest.NewRequest(http.MethodGet, "/plans/new", nil), http.StatusOK,
		"scheduling_plan_form", map[string]any{
			"ServiceTypes": []ServiceType{{ID: "sunday", Name: "Sunday service"}},
			"NextServiceWindows": map[string]Plan{
				"sunday": {ServiceAt: start, ServiceEndsAt: end},
			},
			"CalendarDateProvided": false,
			"IsNew":                true,
			"Plan":                 Plan{ServiceTypeID: "sunday", ServiceAt: start, ServiceEndsAt: end},
		})
	if response.Code != http.StatusOK {
		t.Fatalf("render status = %d, want %d", response.Code, http.StatusOK)
	}
	for _, expected := range []string{
		`data-next-date="2026-11-08"`,
		`data-next-start="09:00"`,
		`data-next-end="11:00"`,
		`data-calendar-date="false"`,
	} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("plan form is missing %q", expected)
		}
	}
}
