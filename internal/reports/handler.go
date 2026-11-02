package reports

import (
	"encoding/csv"
	"fmt"
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

type CCLIReportRow struct {
	Title      string
	Author     string
	CCLI       string
	UsageCount int
	FirstUsed  time.Time
	LastUsed   time.Time
}

// getCCLIData runs the aggregation query.
func (h *Handler) getCCLIData(r *http.Request, start, end string) ([]CCLIReportRow, error) {
	rows, err := h.db.Query(r.Context(), `
		SELECT s.title, s.author, s.ccli_number, COUNT(DISTINCT pl.id), MIN(pl.service_at), MAX(pl.service_at)
		FROM songs s
		JOIN song_arrangements a ON a.song_id = s.id
		JOIN plan_items pi ON pi.arrangement_id = a.id
		JOIN plans pl ON pl.id = pi.plan_id
		WHERE s.ccli_number <> ''
		  AND pl.service_at >= $1 AND pl.service_at <= $2
		GROUP BY s.id, s.title, s.author, s.ccli_number
		ORDER BY COUNT(DISTINCT pl.id) DESC, s.title
	`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CCLIReportRow
	for rows.Next() {
		var c CCLIReportRow
		if err := rows.Scan(&c.Title, &c.Author, &c.CCLI, &c.UsageCount, &c.FirstUsed, &c.LastUsed); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (h *Handler) CCLI(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")

	if start == "" {
		start = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")
	}
	if end == "" {
		end = time.Date(now.Year(), 12, 31, 23, 59, 59, 0, time.Local).Format("2006-01-02")
	}

	data, err := h.getCCLIData(r, start, end)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	h.renderer.Render(w, r, http.StatusOK, "reports_ccli", map[string]any{
		"Start": start,
		"End":   end,
		"Rows":  data,
	})
}

// CCLICsv streams the same data as a CSV download.
func (h *Handler) CCLICsv(w http.ResponseWriter, r *http.Request) {
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")

	data, err := h.getCCLIData(r, start, end)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="ccli_report_%s.csv"`, start))

	cw := csv.NewWriter(w)
	defer cw.Flush()

	_ = cw.Write([]string{"Title", "Author", "CCLI", "Usage Count", "First Used", "Last Used"})
	for _, row := range data {
		_ = cw.Write([]string{
			row.Title,
			row.Author,
			row.CCLI,
			strconv.Itoa(row.UsageCount),
			row.FirstUsed.Format("2006-01-02"),
			row.LastUsed.Format("2006-01-02"),
		})
	}
}
