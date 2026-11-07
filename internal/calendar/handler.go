package calendar

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

// First day of the week in the grid. Change to time.Monday for Monday-first;
// the header labels and the grid both follow this one setting.
const weekStart = time.Sunday

func dayNames() []string {
	names := make([]string, 7)
	for i := range names {
		names[i] = time.Weekday((int(weekStart) + i) % 7).String()[:3]
	}
	return names
}

type Handler struct {
	db       *pgxpool.Pool
	renderer *httpx.Renderer
	feed     *FeedHandler
	people   PeopleLister
}

type Day struct {
	Date      time.Time
	IsToday   bool
	IsOther   bool
	Plans     []Plan
	Blockouts []BlockoutRow
}

type Plan struct {
	ID        string
	Title     string
	TimeStr   string
	URL       string // used by index.html when CanOpenPlans is true
	Published bool
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
	gridStart, weeks := gridBounds(firstOfMonth, weekStart)
	gridEnd := gridStart.AddDate(0, 0, weeks*7) // exclusive

	plansByDay, err := h.plansBetween(r.Context(), gridStart, gridEnd, loc)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	// Blockouts are filtered in SQL for THIS viewer (see blockoutsInRange).
	var me viewer
	var rows []BlockoutRow
	if u, ok := auth.UserFrom(r.Context()); ok {
		me = viewer{ID: u.ID, Role: u.Role}
		rows, err = h.blockoutsInRange(r.Context(), me, gridStart, gridEnd)
		if err != nil {
			httpx.ServerError(w, err)
			return
		}
	}
	byDay := blockoutsByDay(rows, gridStart, gridEnd.AddDate(0, 0, -1))

	var list []BlockoutRow // the table under the grid: this month only
	for _, b := range rows {
		if overlapsRange(b.Starts, b.Ends, firstOfMonth, firstOfNext) {
			list = append(list, b)
		}
	}

	var people []Person
	if canPickPerson(me.Role) {
		if people, err = h.people.ListActive(r.Context()); err != nil {
			slog.Error("calendar people list", "err", err)
			people = nil // the page still works; the dropdown just won't appear
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
			Blockouts: byDay[k],
		})
	}

	prev := firstOfMonth.AddDate(0, -1, 0)
	next := firstOfMonth.AddDate(0, 1, 0)

	h.renderer.Render(w, r, http.StatusOK, "calendar_index", map[string]any{
		"MonthName":          firstOfMonth.Format("January 2006"),
		"MonthValue":         firstOfMonth.Format("2006-01"), // for <input type="month">
		"Year":               firstOfMonth.Year(),
		"MonthNum":           int(firstOfMonth.Month()),
		"DayNames":           dayNames(),
		"PrevYear":           prev.Year(),
		"PrevMonth":          prev.Format("1"),
		"NextYear":           next.Year(),
		"NextMonth":          next.Format("1"),
		"Days":               days,
		"BlockoutList":       list,
		"CanSeeAllBlockouts": me.seesAll(),
		"MeID":               me.ID,
		"People":             people,
		"FeedLinks":          h.feed.linksFor(r),
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
