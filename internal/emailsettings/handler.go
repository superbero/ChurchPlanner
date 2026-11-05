package emailsettings

import (
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
	"github.com/yourchurch/churchplanner/internal/platform/mailer"
)

var validSMTPHost = regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}$`)

type Handler struct {
	mailer   *mailer.Mailer
	renderer *httpx.Renderer
}

func New(mailer *mailer.Mailer, renderer *httpx.Renderer) *Handler {
	return &Handler{mailer: mailer, renderer: renderer}
}

func (h *Handler) View(w http.ResponseWriter, r *http.Request) {
	settings, configured, err := h.mailer.CurrentSMTPSettings(r.Context())
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	h.renderer.Render(w, r, http.StatusOK, "emailsettings_view", map[string]any{
		"Host":        settings.Host,
		"Port":        settings.Port,
		"Username":    settings.Username,
		"FromAddress": settings.FromAddress,
		"Configured":  configured,
		"PasswordSet": settings.Password != "",
		"Saved":       r.URL.Query().Get("saved") == "1",
		"TestSent":    r.URL.Query().Get("test") == "sent",
	})
}

func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid SMTP settings form.", http.StatusBadRequest)
		return
	}
	port, err := strconv.Atoi(strings.TrimSpace(r.FormValue("port")))
	if err != nil || port < 1 || port > 65535 {
		http.Error(w, "Enter a valid SMTP port.", http.StatusUnprocessableEntity)
		return
	}
	host := strings.TrimSpace(r.FormValue("host"))
	username := strings.TrimSpace(r.FormValue("username"))
	from := strings.TrimSpace(r.FormValue("from_address"))
	password := r.FormValue("password")
	if !validSMTPHost.MatchString(host) {
		http.Error(w, "Enter a valid SMTP host.", http.StatusUnprocessableEntity)
		return
	}
	if len(password) > 1024 || len(username) > 320 {
		http.Error(w, "SMTP credentials are too long.", http.StatusUnprocessableEntity)
		return
	}
	parsedFrom, err := mail.ParseAddress(from)
	if err != nil || parsedFrom.Address != from || strings.ContainsAny(from, "\r\n") {
		http.Error(w, "Enter a valid sender email address.", http.StatusUnprocessableEntity)
		return
	}
	settings := mailer.SMTPSettings{
		Host:        host,
		Port:        port,
		Username:    username,
		Password:    password,
		FromAddress: from,
	}
	if err := h.mailer.SaveSMTPSettings(r.Context(), settings); err != nil {
		if errors.Is(err, mailer.ErrEncryptionKeyNeeded) {
			http.Error(w, "Set APP_ENCRYPTION_KEY in the application environment before saving SMTP credentials.", http.StatusServiceUnavailable)
			return
		}
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/settings/email?saved=1", http.StatusSeeOther)
}

func (h *Handler) SendTest(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid test email form.", http.StatusBadRequest)
		return
	}
	to := strings.TrimSpace(r.FormValue("test_email"))
	parsed, err := mail.ParseAddress(to)
	if err != nil || parsed.Address != to {
		http.Error(w, "Enter a valid test email address.", http.StatusUnprocessableEntity)
		return
	}
	if err := h.mailer.SendTest(r.Context(), to); err != nil {
		if errors.Is(err, mailer.ErrSMTPNotConfigured) {
			http.Error(w, "Save SMTP settings before sending a test email.", http.StatusConflict)
			return
		}
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/settings/email?test=sent", http.StatusSeeOther)
}
