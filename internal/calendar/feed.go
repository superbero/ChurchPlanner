package calendar

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

const (
	feedName = "My church schedule"
	tokenLen = 43 // base64url (no padding) of 32 random bytes
)

// FeedSource returns the events one person is scheduled for.
// It is an interface so the calendar module does not have to read the plans
// and assignments tables itself (README module rules).
type FeedSource interface {
	// TODO: change userID to your real ID type once I've seen the auth package.
	AssignmentsFor(ctx context.Context, userID any, from, to time.Time) ([]Event, error)
}

type FeedHandler struct {
	db       *pgxpool.Pool
	renderer *httpx.Renderer
	source   FeedSource
	baseURL  string // PUBLIC_BASE_URL, e.g. https://planner.mychurch.org
}

func NewFeedHandler(db *pgxpool.Pool, r *httpx.Renderer, src FeedSource, baseURL string) *FeedHandler {
	return &FeedHandler{db: db, renderer: r, source: src, baseURL: baseURL}
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Serve is the PUBLIC feed endpoint. The unguessable token in the URL is the
// only credential, because calendar apps cannot log in. Register it OUTSIDE
// session/auth middleware.
func (f *FeedHandler) Serve(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSuffix(chi.URLParam(r, "token"), ".ics")
	if len(token) != tokenLen {
		http.NotFound(w, r)
		return
	}

	var userID string
	err := f.db.QueryRow(r.Context(),
		`SELECT user_id::text FROM calendar_feed_tokens WHERE token = $1`, token).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	now := time.Now()
	events, err := f.source.AssignmentsFor(r.Context(), userID, now.AddDate(0, 0, -30), now.AddDate(1, 0, 0))
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	body := BuildCalendar(feedName, events)
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`

	h := w.Header()
	h.Set("Content-Type", "text/calendar; charset=utf-8")
	h.Set("Content-Disposition", `inline; filename="schedule.ics"`)
	h.Set("Cache-Control", "private, max-age=300")
	h.Set("ETag", etag)
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Robots-Tag", "noindex, nofollow")

	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(body)
}

// ---- authenticated pages -------------------------------------------------

type SubscribeLinks struct {
	HTTPS       string       // paste-able address (Google "From URL", Outlook "Subscribe from web")
	Webcal      template.URL // webcal:// needs template.URL or html/template rewrites it to #ZgotmplZ
	Google      template.URL
	Outlook365  template.URL
	OutlookLive template.URL
}

func subscribeLinks(baseURL, token, calName string) SubscribeLinks {
	httpsURL := strings.TrimRight(baseURL, "/") + "/calendar/feed/" + token + ".ics"
	rest := strings.TrimPrefix(strings.TrimPrefix(httpsURL, "https://"), "http://")
	webcal := "webcal://" + rest
	q := url.QueryEscape
	return SubscribeLinks{
		HTTPS:       httpsURL,
		Webcal:      template.URL(webcal),
		Google:      template.URL("https://calendar.google.com/calendar/render?cid=" + q(webcal)),
		Outlook365:  template.URL("https://outlook.office.com/calendar/0/addfromweb?name=" + q(calName) + "&url=" + q(webcal)),
		OutlookLive: template.URL("https://outlook.live.com/calendar/0/addfromweb?name=" + q(calName) + "&url=" + q(webcal)),
	}
}

func (f *FeedHandler) base(r *http.Request) string {
	if f.baseURL != "" {
		return f.baseURL
	}
	return "https://" + r.Host // fallback; set PUBLIC_BASE_URL in production
}

func (f *FeedHandler) Settings(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Sign in required", http.StatusUnauthorized)
		return
	}
	var token string
	err := f.db.QueryRow(r.Context(),
		`SELECT token FROM calendar_feed_tokens WHERE user_id = $1`, u.ID).Scan(&token)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		httpx.ServerError(w, err)
		return
	}

	data := map[string]any{"HasFeed": token != ""}
	if token != "" {
		data["Links"] = subscribeLinks(f.base(r), token, feedName)
	}
	f.renderer.Render(w, r, http.StatusOK, "calendar_feed", data)
}

// Regenerate creates the feed on first use, and replaces the token afterwards.
// The old URL stops working immediately, which is the "revoke" mechanism.
func (f *FeedHandler) Regenerate(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Sign in required", http.StatusUnauthorized)
		return
	}
	token, err := newToken()
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	if _, err := f.db.Exec(r.Context(), `
		INSERT INTO calendar_feed_tokens (user_id, token) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET token = EXCLUDED.token, created_at = now()
	`, u.ID, token); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/calendar/feed", http.StatusSeeOther)
}

func (f *FeedHandler) Disable(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Sign in required", http.StatusUnauthorized)
		return
	}
	if _, err := f.db.Exec(r.Context(),
		`DELETE FROM calendar_feed_tokens WHERE user_id = $1`, u.ID); err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/calendar/feed", http.StatusSeeOther)
}
