package scheduling

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

func TestPlanViewRendersItemCopyAndSafeTimeEditControls(t *testing.T) {
	renderer, err := httpx.NewRenderer(fstest.MapFS{
		"layout.html": &fstest.MapFile{
			Data: []byte(`{{define "layout.html"}}{{template "content" .}}{{end}}`),
		},
	})
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}
	if err := renderer.Add("scheduling", Templates(), "plan_view.html"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	start := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	response := httptest.NewRecorder()
	renderer.Render(response, httptest.NewRequest(http.MethodGet, "/plans/plan-1", nil),
		http.StatusOK, "scheduling_plan_view", map[string]any{
			"Plan": Plan{
				ID: "plan-1", Title: "Sunday", ServiceAt: start,
				ServiceEndsAt: start.Add(90 * time.Minute),
			},
			"Times": []PlanTimeWithItems{{
				Time: PlanTime{
					ID: "time-1", Kind: "service", Name: "Children's Service",
					StartsAt: start, EndsAt: start.Add(90 * time.Minute),
				},
				Items: []PlanItem{
					{ID: "item-1", Kind: "header", Title: "Welcome"},
					{ID: "item-2", Kind: "scripture", Title: "John 3:16", Notes: "For God so loved the world.\n<script>alert(1)</script>"},
				},
			}},
			"NextStart":             start,
			"Teams":                 []TeamWithPositions{},
			"ManageablePositions":   []Position{},
			"ManageablePositionIDs": map[string]bool{},
			"UsersByPos":            map[string][]auth.User{},
			"BlockedUsers":          map[string]bool{},
		})
	if response.Code != http.StatusOK {
		t.Fatalf("render status = %d, want %d; body: %s", response.Code, http.StatusOK, response.Body.String())
	}
	body := response.Body.String()
	for _, expected := range []string{
		`edit-time-button`,
		`data-time-name="Children&#39;s Service"`,
		`action="/plans/plan-1/items/item-1/duplicate"`,
		`title="Copy this item to the end of this service section"`,
		`+ Add Scripture`,
		`World English Bible (WEB)`,
		`For God so loved the world.`,
		`&lt;script&gt;alert(1)&lt;/script&gt;`,
		`class="scripture-toggle item-scripture"`,
		`aria-expanded="false"`,
		`class="scripture-text scripture-item-text" hidden`,
		`button.setAttribute('aria-expanded', String(!expanded))`,
		`https://bible-api.com/`,
		`id="scripture-book"`,
		`id="scripture-chapter"`,
		`id="scripture-start-verse"`,
		`id="scripture-end-verse"`,
		`class="scripture-picker-grid"`,
		`scriptureReference.addEventListener('keydown'`,
		`loadSelectedChapter();`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("plan view is missing %q", expected)
		}
	}
	if strings.Contains(body, "onclick=\"openEditTimeModal(") {
		t.Error("time edit control still embeds user-provided data in an inline JavaScript handler")
	}
}
