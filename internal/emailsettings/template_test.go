package emailsettings

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

func TestEmailSettingsTemplateDoesNotRenderSMTPPassword(t *testing.T) {
	renderer, err := httpx.NewRenderer(fstest.MapFS{
		"layout.html": &fstest.MapFile{
			Data: []byte(`{{define "layout.html"}}{{template "content" .}}{{end}}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := renderer.Add("emailsettings", Templates(), "*.html"); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	renderer.Render(response, httptest.NewRequest(http.MethodGet, "/settings/email", nil), http.StatusOK,
		"emailsettings_view", map[string]any{
			"Host": "smtp.gmail.com", "Port": 587, "Username": "admin@example.com",
			"FromAddress": "admin@example.com", "Configured": true, "PasswordSet": true,
			"Password": "smtp-secret-must-not-render",
		})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	for _, want := range []string{"SMTP host", "SMTP app password", "Save SMTP settings", "Send test email"} {
		if !strings.Contains(response.Body.String(), want) {
			t.Errorf("settings page missing %q", want)
		}
	}
	if strings.Contains(response.Body.String(), "smtp-secret-must-not-render") {
		t.Error("settings page must not render a stored SMTP password")
	}
}
