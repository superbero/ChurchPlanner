package calendar

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

type Handler struct {
	db       *pgxpool.Pool
	renderer *httpx.Renderer
}

type Day struct {
	Date     time.Time
	IsToday  bool
	IsOther  bool // padding days before/after the month
	Plans    []Plan
}

type Plan struct {
	ID        string
	Title     string
	TimeStr   string
	Published bool
}

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	// Default to current month
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	month, _ := strconv.Atoi(r.URL.Query().Get("month"))

	now := time.Now()
	if year == 0 || month == 0 {
		year = now.Year()
		month = int(now.Month())
	}

	firstOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	firstOfNext := firstOfMonth.AddDate(0, 1, 0)

	// Fetch plans for the month
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
	defer rows.Close()

	plansByDay := make(map[int][]Plan)
	for rows.Next() {
		var p Plan
		var date time.Time
		if err := rows.Scan(&p.ID, &p.Title, &date, &p.Published); err != nil {
			httpx.ServerError(w, err)
			return
		}
		p.TimeStr = date.Format("15:04")
		day := date.Day()
		plansByDay[day] = append(plansByDay[day], p)
	}

	// Build the grid (start on Sunday)
	// Go's Weekday(): Sunday=0, Monday=1...
	padding := int(firstOfMonth.Weekday())
	gridStart := firstOfMonth.AddDate(0, 0, -padding)

	var days []Day
	for i := 0; i < 42; i++ { // 6 weeks to cover all months
		d := gridStart.AddDate(0, 0, i)
		isOther := d.Month() != firstOfMonth.Month()

		var plans []Plan
		if !isOther {
			plans = plansByDay[d.Day()]
		}

		days = append(days, Day{
			Date:    d,
			IsToday: d.Format("2006-01-02") == now.Format("2006-01-02"),
			IsOther: isOther,
			Plans:   plans,
		})
	}

	h.renderer.Render(w, r, http.StatusOK, "calendar_index", map[string]any{
		"MonthName": firstOfMonth.Format("January 2006"),
		"PrevMonth": firstOfMonth.AddDate(0, -1, 0),
		"NextMonth": firstOfMonth.AddDate(0, 1, 0),
		"Days":      days,
	})
}
