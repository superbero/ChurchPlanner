package people

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
	"github.com/yourchurch/churchplanner/internal/platform/httpx"
	"github.com/yourchurch/churchplanner/internal/scheduling"
)

func newPeopleTestRenderer(t *testing.T) *httpx.Renderer {
	t.Helper()
	renderer, err := httpx.NewRenderer(fstest.MapFS{
		"layout.html": &fstest.MapFile{
			Data: []byte(`{{define "layout.html"}}{{template "content" .}}{{end}}`),
		},
	})
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}
	if err := renderer.Add("people", Templates(), "*.html"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	return renderer
}

func TestPeopleListUsesCurrentViewModelAndEditRoute(t *testing.T) {
	renderer := newPeopleTestRenderer(t)
	response := httptest.NewRecorder()
	renderer.Render(response, httptest.NewRequest(http.MethodGet, "/people", nil), http.StatusOK,
		"people_list", map[string]any{
			"Users": []UserWithSkills{{
				User: auth.User{
					ID: "user-1", Name: "Alex Doe", Title: "Worship Director",
					Role: auth.RoleEditor, Email: "alex@example.com", Phone: "555-0100",
					Status: "active",
				},
				Skills:    "Vocals, Guitar",
				TeamRoles: "Worship (leader)",
			}},
			"Query":        "",
			"StatusFilter": "",
		})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	for _, want := range []string{
		"Alex Doe", "Worship Director", "Worship (leader)",
		"Vocals", "Guitar", "alex@example.com", `href="/people/user-1"`,
	} {
		if !strings.Contains(response.Body.String(), want) {
			t.Errorf("people list missing %q", want)
		}
	}
	for _, wrong := range []string{".People", ".SystemRole", "/people/user-1/edit"} {
		if strings.Contains(response.Body.String(), wrong) {
			t.Errorf("people list contains stale output %q", wrong)
		}
	}
}

func TestPeopleEditTemplateUsesUserStatusTitleAndMemberships(t *testing.T) {
	renderer := newPeopleTestRenderer(t)
	response := httptest.NewRecorder()
	renderer.Render(response, httptest.NewRequest(http.MethodGet, "/people/user-1", nil), http.StatusOK,
		"people_view", map[string]any{
			"User": auth.User{
				ID: "user-1", Name: "Alex Doe", Title: "Worship Director",
				Email: "alex@example.com", Role: auth.RoleEditor, Status: "suspended",
				Phone: "+15550100",
			},
			"Positions": []scheduling.Position{
				{ID: "position-1", Name: "Drums", TeamName: "Worship"},
				{ID: "position-2", Name: "Guitar", TeamName: "Worship"},
			},
			"SkillMap":                 map[string]bool{"position-1": true},
			"Teams":                    []scheduling.Team{},
			"MembershipRoles":          map[string]string{},
			"CanAssignPrivilegedRoles": false,
		})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	for _, want := range []string{
		`action="/people/user-1"`, `name="title" value="Worship Director"`,
		`type="email" name="email" value="alex@example.com" required maxlength="254"`,
		`readonly`,
		`value="suspended" selected`, `value="editor"`,
		`type="tel" name="phone"`, `placeholder="+1 555-0100"`,
		`pattern="\+[1-9][0-9\s().-]{5,24}"`,
		`id="skill-picker"`, `id="add-skill-btn"`, `remove-skill`,
		`name="position_ids"`, `picker.addEventListener('change'`,
	} {
		if !strings.Contains(response.Body.String(), want) {
			t.Errorf("people edit page missing %q", want)
		}
	}

	for _, wrong := range []string{`value="volunteer"`, `value="guest"`} {
		if strings.Contains(response.Body.String(), wrong) {
			t.Errorf("people edit page contains obsolete role option %q", wrong)
		}
	}
}

func TestPeopleAdminCanEditEmail(t *testing.T) {
	renderer := newPeopleTestRenderer(t)
	response := httptest.NewRecorder()
	renderer.Render(response, httptest.NewRequest(http.MethodGet, "/people/user-1", nil), http.StatusOK,
		"people_view", map[string]any{
			"User":                     auth.User{ID: "user-1", Name: "Alex Doe", Email: "alex@example.com"},
			"Positions":                []scheduling.Position{},
			"SkillMap":                 map[string]bool{},
			"Teams":                    []scheduling.Team{},
			"MembershipRoles":          map[string]string{},
			"CanAssignPrivilegedRoles": true,
		})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), `type="email" name="email" value="alex@example.com" required maxlength="254"`) ||
		strings.Contains(response.Body.String(), `readonly`) {
		t.Errorf("admin email field is not editable")
	}
}
