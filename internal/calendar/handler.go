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
	ID     string
	Starts time.Time
	Ends   time.Time
	Reason string
}

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	year, month, err := requestedMonth(
		r.URL.Query().Get("year"), r.URL.Query().Get("month"), now)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	u, ok := auth.UserFrom(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	canManagePlans := u.Role == "admin"
	if !canManagePlans {
		if err := h.db.QueryRow(r.Context(), `
			SELECT EXISTS(
				SELECT 1 FROM team_memberships
				WHERE user_id = $1 AND role = 'leader'
			)
		`, u.ID).Scan(&canManagePlans); err != nil {
			httpx.ServerError(w, err)
			return
		}
	}

	firstOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	firstOfNext := firstOfMonth.AddDate(0, 1, 0)

	// Volunteers only see plans once published; leaders and admins can see drafts.
	query := `
		SELECT id, title, service_at, published
		FROM plans
		WHERE service_at >= $1 AND service_at < $2 AND published = TRUE
		ORDER BY service_at
	`
	if canManagePlans {
		query = `
			SELECT id, title, service_at, published
			FROM plans
			WHERE service_at >= $1 AND service_at < $2
			ORDER BY service_at
		`
	}

	rows, err := h.db.Query(r.Context(), query, firstOfMonth, firstOfNext)
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
	if err := rows.Err(); err != nil {
		rows.Close()
		httpx.ServerError(w, err)
		return
	}
	rows.Close()

	// Availability is private to the volunteer; schedulers see conflicts when
	// assigning people to a specific plan.
	brows, err := h.db.Query(r.Context(), `
		SELECT id, starts_on, ends_on, reason
		FROM user_blockouts
		WHERE user_id = $1 AND starts_on < $2 AND ends_on >= $3
		ORDER BY starts_on
	`, u.ID, firstOfNext, firstOfMonth)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	var myBlockouts []Blockout
	blockoutsByDay := make(map[int][]Blockout)
	for brows.Next() {
		var b Blockout
		if err := brows.Scan(&b.ID, &b.Starts, &b.Ends, &b.Reason); err != nil {
			brows.Close()
			httpx.ServerError(w, err)
			return
		}
		myBlockouts = append(myBlockouts, b)
		start := b.Starts
		end := b.Ends
		for current := start; !current.After(end); current = current.AddDate(0, 0, 1) {
			if current.Year() == year && int(current.Month()) == month {
				blockoutsByDay[current.Day()] = append(blockoutsByDay[current.Day()], b)
			}
		}
	}
	if err := brows.Err(); err != nil {
		brows.Close()
		httpx.ServerError(w, err)
		return
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

	h.renderer.Render(w, r, http.StatusOK, "calendar_index", map[string]any{
		"MonthName":      firstOfMonth.Format("January 2006"),
		"PrevMonth":      firstOfMonth.AddDate(0, -1, 0),
		"NextMonth":      firstOfMonth.AddDate(0, 1, 0),
		"Days":           days,
		"MyBlockouts":    myBlockouts,
		"CanManagePlans": canManagePlans,
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
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
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
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	tag, err := h.db.Exec(r.Context(), `
		DELETE FROM user_blockouts WHERE id = $1 AND user_id = $2
	`, chi.URLParam(r, "id"), u.ID)
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
