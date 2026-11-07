package calendar

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

const maxReasonLen = 200

type Handler struct {
	db       *pgxpool.Pool
	renderer *httpx.Renderer
}

type Day struct {
	Date      time.Time
	IsToday   bool
	IsOther   bool
	Plans     []Plan
	Blockouts []Blockout
}

type Plan struct {
	ID        string
	Title     string
	TimeStr   string
	URL       string // used by index.html when CanOpenPlans is true
	Published bool
}

type Blockout struct {
	ID       string
	UserName string
	Reason   string
}

// MyBlockout feeds the "Your unavailable dates" list in index.html.
type MyBlockout struct {
	ID     string
	Starts time.Time
	Ends   time.Time
	Reason string
}

// location returns the church's time zone.
// TODO: load from an org setting; time.Local is whatever the container uses (often UTC).
func (h *Handler) location() *time.Location { return time.Local }

// canSeeDrafts reports whether the current user may see unpublished plans.
// TODO: wire to your auth package (editor/admin, or team leader of that plan).
// Returning true keeps today's behaviour, which shows drafts to everyone.
func canSeeDrafts(ctx context.Context) bool { return true }

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	loc := h.location()
	now := time.Now().In(loc)

	year, month, err := requestedMonth(
		r.URL.Query().Get("year"), r.URL.Query().Get("month"), now)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	firstOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
	firstOfNext := firstOfMonth.AddDate(0, 1, 0)
	gridStart, weeks := gridBounds(firstOfMonth, time.Sunday)
	gridEnd := gridStart.AddDate(0, 0, weeks*7) // exclusive

	plansByDay, err := h.plansBetween(r.Context(), gridStart, gridEnd, loc)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	blockoutsByDay, err := h.blockoutsBetween(r.Context(), gridStart, gridEnd)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	var mine []MyBlockout
	if u, ok := auth.UserFrom(r.Context()); ok {
		mine, err = h.myBlockouts(r.Context(), u.ID, firstOfMonth, firstOfNext)
		if err != nil {
			httpx.ServerError(w, err)
			return
		}
	}

	todayKey := dateKey(now)
	days := make([]Day, 0, weeks*7)
	for i := 0; i < weeks*7; i++ {
		d := gridStart.AddDate(0, 0, i)
		k := dateKey(d)
		days = append(days, Day{
			Date:      d,
			IsToday:   k == todayKey,
			IsOther:   d.Month() != firstOfMonth.Month(),
			Plans:     plansByDay[k],
			Blockouts: blockoutsByDay[k],
		})
	}

	prev := firstOfMonth.AddDate(0, -1, 0)
	next := firstOfMonth.AddDate(0, 1, 0)

	h.renderer.Render(w, r, http.StatusOK, "calendar_index", map[string]any{
		"MonthName":   firstOfMonth.Format("January 2006"),
		"PrevYear":    prev.Year(),
		"PrevMonth":   prev.Format("1"),
		"NextYear":    next.Year(),
		"NextMonth":   next.Format("1"),
		"Days":        days,
		"MyBlockouts": mine,
	})
}

func (h *Handler) plansBetween(ctx context.Context, from, to time.Time, loc *time.Location) (map[string][]Plan, error) {
	rows, err := h.db.Query(ctx, `
		SELECT id, title, service_at, published
		FROM plans
		WHERE service_at >= $1 AND service_at < $2
		  AND ($3 OR published)
		ORDER BY service_at
	`, from, to, canSeeDrafts(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string][]Plan)
	for rows.Next() {
		var p Plan
		var at time.Time
		if err := rows.Scan(&p.ID, &p.Title, &at, &p.Published); err != nil {
			return nil, err
		}
		at = at.In(loc)
		p.TimeStr = at.Format("15:04")
		p.URL = "/plans/" + p.ID
		out[dateKey(at)] = append(out[dateKey(at)], p)
	}
	return out, rows.Err()
}

func (h *Handler) blockoutsBetween(ctx context.Context, from, to time.Time) (map[string][]Blockout, error) {
	rows, err := h.db.Query(ctx, `
		SELECT b.id, u.name, b.starts_on, b.ends_on, b.reason
		FROM user_blockouts b
		JOIN users u ON u.id = b.user_id
		WHERE b.starts_on < $2::date AND b.ends_on >= $1::date
		ORDER BY u.name
	`, dateKey(from), dateKey(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lastDay := to.AddDate(0, 0, -1)
	out := make(map[string][]Blockout)
	for rows.Next() {
		var id, name, reason string
		var start, end time.Time
		if err := rows.Scan(&id, &name, &start, &end, &reason); err != nil {
			return nil, err
		}
		// Clamped to the visible grid, so a year-long blockout is cheap.
		for _, day := range daysInRange(start, end, from, lastDay) {
			k := dateKey(day)
			out[k] = append(out[k], Blockout{ID: id, UserName: name, Reason: reason})
		}
	}
	return out, rows.Err()
}

func (h *Handler) myBlockouts(ctx context.Context, userID any, from, to time.Time) ([]MyBlockout, error) {
	rows, err := h.db.Query(ctx, `
		SELECT id, starts_on, ends_on, reason
		FROM user_blockouts
		WHERE user_id = $1 AND starts_on < $3::date AND ends_on >= $2::date
		ORDER BY starts_on
	`, userID, dateKey(from), dateKey(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MyBlockout
	for rows.Next() {
		var b MyBlockout
		if err := rows.Scan(&b.ID, &b.Starts, &b.Ends, &b.Reason); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func requestedMonth(yearValue, monthValue string, now time.Time) (int, int, error) {
	if yearValue == "" && monthValue == "" {
		return now.Year(), int(now.Month()), nil
	}
	if yearValue == "" || monthValue == "" {
		return 0, 0, fmt.Errorf("year and month are both required")
	}
	year, err := strconv.Atoi(yearValue)
	if err != nil {
		return 0, 0, fmt.Errorf("year and month must be valid numbers")
	}
	month, err := strconv.Atoi(monthValue)
	if err != nil {
		return 0, 0, fmt.Errorf("year and month must be valid numbers")
	}
	if month < 1 || month > 12 || year < 1 || year > 9999 {
		return 0, 0, fmt.Errorf("year or month is out of range")
	}
	return year, month, nil
}

func (h *Handler) AddBlockout(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Sign in required", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad form", http.StatusBadRequest)
		return
	}
	startStr := r.PostFormValue("starts_on")
	endStr := r.PostFormValue("ends_on")
	reason := strings.TrimSpace(r.PostFormValue("reason"))

	if startStr == "" || endStr == "" {
		http.Error(w, "Start and end dates required", http.StatusBadRequest)
		return
	}
	if utf8.RuneCountInString(reason) > maxReasonLen {
		http.Error(w, "Reason is too long", http.StatusBadRequest)
		return
	}

	start, err1 := time.Parse("2006-01-02", startStr)
	end, err2 := time.Parse("2006-01-02", endStr)
	if err1 != nil || err2 != nil || end.Before(start) {
		http.Error(w, "Invalid dates", http.StatusBadRequest)
		return
	}

	if _, err := h.db.Exec(r.Context(), `
		INSERT INTO user_blockouts (user_id, starts_on, ends_on, reason)
		VALUES ($1, $2, $3, $4)
	`, u.ID, start, end, reason); err != nil {
		httpx.ServerError(w, err)
		return
	}

	http.Redirect(w, r, "/calendar?year="+start.Format("2006")+"&month="+start.Format("1"), http.StatusSeeOther)
}

func (h *Handler) DeleteBlockout(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Sign in required", http.StatusUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	// Scoped to the owner: without "AND user_id" any signed-in user could
	// delete anyone's blockout by guessing/iterating IDs.
	// TODO: allow managers/admins to delete others' blockouts if you want that.
	tag, err := h.db.Exec(r.Context(),
		`DELETE FROM user_blockouts WHERE id = $1 AND user_id = $2`, id, u.ID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, "/calendar", http.StatusSeeOther)
}
