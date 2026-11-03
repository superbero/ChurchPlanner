package calendar

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

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
	Published bool
}

type Blockout struct {
	ID       string
	UserName string
	Reason   string
}

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	year, month, err := requestedMonth(
		r.URL.Query().Get("year"), r.URL.Query().Get("month"), now)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	firstOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	firstOfNext := firstOfMonth.AddDate(0, 1, 0)

	// 1. Fetch plans
	rows, err := h.db.Query(r.Context(), `
		SELECT id, title, service_at, published
		FROM plans
		WHERE service_at >= $1 AND service_at < $2
		ORDER BY service_at
	`, firstOfMonth, firstOfNext)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	plansByDay := make(map[int][]Plan)
	for rows.Next() {
		var p Plan
		var date time.Time
		if err := rows.Scan(&p.ID, &p.Title, &date, &p.Published); err != nil {
			rows.Close()
			httpx.ServerError(w, err)
			return
		}
		p.TimeStr = date.Format("15:04")
		plansByDay[date.Day()] = append(plansByDay[date.Day()], p)
	}
	rows.Close()

	// 2. Fetch blockouts
	brows, err := h.db.Query(r.Context(), `
		SELECT b.id, u.name, b.starts_on, b.ends_on, b.reason
		FROM user_blockouts b
		JOIN users u ON u.id = b.user_id
		WHERE b.starts_on < $2 AND b.ends_on >= $1
	`, firstOfMonth, firstOfNext)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	blockoutsByDay := make(map[int][]Blockout)
	for brows.Next() {
		var id, name, reason string
		var start, end time.Time
		if err := brows.Scan(&id, &name, &start, &end, &reason); err != nil {
			brows.Close()
			httpx.ServerError(w, err)
			return
		}

		curr := start
		for !curr.After(end) {
			if curr.Month() == firstOfMonth.Month() && curr.Year() == firstOfMonth.Year() {
				blockoutsByDay[curr.Day()] = append(blockoutsByDay[curr.Day()], Blockout{
					ID: id, UserName: name, Reason: reason,
				})
			}
			curr = curr.AddDate(0, 0, 1)
		}
	}

	brows.Close()

	// 3. Build the grid
	padding := int(firstOfMonth.Weekday())
	gridStart := firstOfMonth.AddDate(0, 0, -padding)

	var days []Day
	for i := 0; i < 42; i++ {
		d := gridStart.AddDate(0, 0, i)
		isOther := d.Month() != firstOfMonth.Month()

		var plans []Plan
		var blockouts []Blockout
		if !isOther {
			plans = plansByDay[d.Day()]
			blockouts = blockoutsByDay[d.Day()]
		}

		days = append(days, Day{
			Date:      d,
			IsToday:   d.Format("2006-01-02") == now.Format("2006-01-02"),
			IsOther:   isOther,
			Plans:     plans,
			Blockouts: blockouts,
		})
	}

	prev := firstOfMonth.AddDate(0, -1, 0)
	next := firstOfMonth.AddDate(0, 1, 0)

	h.renderer.Render(w, r, http.StatusOK, "calendar_index", map[string]any{
		"MonthName": firstOfMonth.Format("January 2006"),
		"PrevYear":  prev.Year(),
		"PrevMonth": prev.Format("1"),
		"NextYear":  next.Year(),
		"NextMonth": next.Format("1"),
		"Days":      days,
	})
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
	u, _ := auth.UserFrom(r.Context())
	startStr := r.FormValue("starts_on")
	endStr := r.FormValue("ends_on")
	reason := strings.TrimSpace(r.FormValue("reason"))

	if startStr == "" || endStr == "" {
		http.Error(w, "Start and end dates required", http.StatusBadRequest)
		return
	}

	start, err1 := time.Parse("2006-01-02", startStr)
	end, err2 := time.Parse("2006-01-02", endStr)
	if err1 != nil || err2 != nil || end.Before(start) {
		http.Error(w, "Invalid dates", http.StatusBadRequest)
		return
	}

	_, err := h.db.Exec(r.Context(), `
		INSERT INTO user_blockouts (user_id, starts_on, ends_on, reason)
		VALUES ($1, $2, $3, $4)
	`, u.ID, start, end, reason)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	http.Redirect(w, r, "/calendar?year="+start.Format("2006")+"&month="+start.Format("1"), http.StatusSeeOther)
}

func (h *Handler) DeleteBlockout(w http.ResponseWriter, r *http.Request) {
	// We get the blockout ID from the URL, usually defined in routes.go as /calendar/blockouts/{id}/delete
	// We need to import "github.com/go-chi/chi/v5" if it's not already there.
	id := chi.URLParam(r, "id")

	_, err := h.db.Exec(r.Context(), `DELETE FROM user_blockouts WHERE id = $1`, id)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	http.Redirect(w, r, "/calendar", http.StatusSeeOther)
}
