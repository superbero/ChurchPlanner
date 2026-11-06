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

	sundayStart := time.Date(2026, time.November, 8, 9, 0, 0, 0, time.UTC)
	sundayEnd := sundayStart.Add(2 * time.Hour)
	midweekStart := time.Date(2026, time.November, 11, 10, 0, 0, 0, time.UTC)
	midweekEnd := midweekStart.Add(90 * time.Minute)
	response := httptest.NewRecorder()
	renderer.Render(response, httptest.NewRequest(http.MethodGet, "/plans/new", nil), http.StatusOK,
		"scheduling_plan_form", map[string]any{
			"ServiceTypes": []ServiceType{
				{ID: "sunday", Name: "Sunday service"},
				{ID: "midweek", Name: "Midweek"},
			},
			"NextServiceWindows": map[string]Plan{
				"sunday":  {ServiceAt: sundayStart, ServiceEndsAt: sundayEnd},
				"midweek": {ServiceAt: midweekStart, ServiceEndsAt: midweekEnd},
			},
			"CalendarDateProvided": false,
			"IsNew":                true,
			"Plan":                 Plan{ServiceTypeID: "midweek", ServiceAt: midweekStart, ServiceEndsAt: midweekEnd},
		})
	if response.Code != http.StatusOK {
		t.Fatalf("render status = %d, want %d", response.Code, http.StatusOK)
	}
	for _, expected := range []string{
		`data-next-date="2026-11-08"`,
		`data-next-start="09:00"`,
		`data-next-end="11:00"`,
		`data-next-date="2026-11-11"`,
		`data-next-start="10:00"`,
		`data-next-end="11:30"`,
		`data-calendar-date="false"`,
	} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("plan form is missing %q", expected)
		}
	}
	if strings.Contains(response.Body.String(), "0001-01-01") {
		t.Error("plan form rendered a zero date")
	}
}

func TestEditPlanFormDoesNotRenderZeroServiceWindow(t *testing.T) {
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

	response := httptest.NewRecorder()
	renderer.Render(response, httptest.NewRequest(http.MethodGet, "/plans/plan-1/edit", nil), http.StatusOK,
		"scheduling_plan_form", map[string]any{
			"ServiceTypes":       []ServiceType{{ID: "sunday", Name: "Sunday service"}},
			"NextServiceWindows": map[string]Plan{},
			"IsNew":              false,
			"Plan":               Plan{ID: "plan-1", ServiceTypeID: "sunday"},
			"Error":              "This plan has an invalid service window.",
			"FormValues":         planFormTimeValues(Plan{}),
		})
	if response.Code != http.StatusOK {
		t.Fatalf("render status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if strings.Contains(body, "0001-01-01") || strings.Contains(body, "0000-12-31") {
		t.Fatal("edit form rendered an invalid zero date")
	}
	if !strings.Contains(body, `name="service_date" id="service-date-input"
                   value=""
                   data-calendar-date="true"`) {
		t.Error("edit form did not render an empty date field for the invalid service window")
	}
	if !strings.Contains(body, "This plan has an invalid service window.") {
		t.Error("edit form did not show the invalid service window error")
	}
	if strings.Contains(body, "data-next-date=") {
		t.Error("edit form unexpectedly included a next-service suggestion")
	}

	start := time.Date(2026, time.November, 22, 9, 30, 0, 0, time.Local)
	plan := Plan{
		ID:            "plan-1",
		ServiceTypeID: "sunday",
		ServiceAt:     start,
		ServiceEndsAt: start.Add(90 * time.Minute),
	}
	response = httptest.NewRecorder()
	renderer.Render(response, httptest.NewRequest(http.MethodGet, "/plans/plan-1/edit", nil), http.StatusOK,
		"scheduling_plan_form", map[string]any{
			"ServiceTypes":       []ServiceType{{ID: "sunday", Name: "Sunday service"}},
			"NextServiceWindows": map[string]Plan{},
			"IsNew":              false,
			"Plan":               plan,
			"FormValues":         planFormTimeValues(plan),
		})
	body = response.Body.String()
	for _, expected := range []string{
		`value="2026-11-22"`,
		`name="service_start" value="09:30"`,
		`name="service_end" value="11:00"`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("edit form did not preserve the service window value %q", expected)
		}
	}
}
