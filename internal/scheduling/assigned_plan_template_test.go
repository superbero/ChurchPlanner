package scheduling

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

func TestAssignedPlanTemplateShowsServiceDetailsWithoutManagementLinks(t *testing.T) {
	renderer, err := httpx.NewRenderer(fstest.MapFS{
		"layout.html": &fstest.MapFile{
			Data: []byte(`{{define "layout.html"}}{{template "content" .}}{{end}}`),
		},
	})
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}
	if err := renderer.Add("scheduling", Templates(),
		"assigned_plan.html"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	start := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	request := httptest.NewRequest(http.MethodGet,
		"/my-schedule/assignment-1", nil)
	response := httptest.NewRecorder()
	renderer.Render(response, request, http.StatusOK,
		"scheduling_assigned_plan", map[string]any{
			"Plan": Plan{
				ID: "plan-1", Title: "Sunday Service",
				ServiceAt: start, ServiceEndsAt: start.Add(90 * time.Minute),
			},
			"Assignment": PlanPerson{
				ID: "assignment-1", Status: "U",
				PositionName: "Keys", TeamName: "Worship Team",
			},
			"Times": []PlanTimeWithItems{{
				Time: PlanTime{
					Name: "Service", StartsAt: start,
					EndsAt: start.Add(90 * time.Minute),
				},
				Items: []PlanItem{{
					Position: 1, Kind: "song", SongID: "song-1",
					SongTitle: "Amazing Grace", ArrangementName: "Acoustic",
					Key: "G", DurationMinutes: 5,
				}},
			}},
		})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	bodyBytes := []byte(body)
	for _, expected := range []string{
		"Sunday Service", "Worship Team · Keys", "Amazing Grace",
		"Accept assignment", "/my-schedule/assignment-1/respond",
	} {
		if !bytes.Contains(bodyBytes, []byte(expected)) {
			t.Errorf("rendered plan is missing %q", expected)
		}
	}
	if bytes.Contains(bodyBytes, []byte("/plans/plan-1/edit")) {
		t.Fatal("assigned plan exposed a plan-management link")
	}
}
