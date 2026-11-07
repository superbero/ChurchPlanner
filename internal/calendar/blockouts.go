package calendar

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/yourchurch/churchplanner/internal/platform/auth"
	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

const (
	maxReasonLen        = 200
	maxBlockoutSpanDays = 5 * 366
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// BlockoutRow is one blockout as shown to ONE specific viewer.
// Mine/CanEdit are computed from the policy, so templates never decide permissions.
type BlockoutRow struct {
	ID       string
	UserID   string
	UserName string
	Reason   string
	AddedBy  string // empty when the person added it themselves
	Starts   time.Time
	Ends     time.Time
	Mine     bool
	CanEdit  bool
}

type viewer struct{ ID, Role string }

func (v viewer) seesAll() bool { return canManageAllBlockouts(v.Role) }

// ---- validation (shared by add and edit) ----------------------------------

type blockoutInput struct {
	Start, End time.Time
	Reason     string
}

func parseBlockoutInput(startStr, endStr, reason string) (blockoutInput, error) {
	startStr, endStr = strings.TrimSpace(startStr), strings.TrimSpace(endStr)
	reason = strings.TrimSpace(reason)

	if startStr == "" || endStr == "" {
		return blockoutInput{}, errors.New("start and end dates are required")
	}
	start, err1 := time.Parse("2006-01-02", startStr)
	end, err2 := time.Parse("2006-01-02", endStr)
	if err1 != nil || err2 != nil {
		return blockoutInput{}, errors.New("dates must look like 2026-11-08")
	}
	if end.Before(start) {
		return blockoutInput{}, errors.New("the end date is before the start date")
	}
	if end.Sub(start) > maxBlockoutSpanDays*24*time.Hour {
		return blockoutInput{}, errors.New("that date range is too long")
	}
	if utf8.RuneCountInString(reason) > maxReasonLen {
		return blockoutInput{}, fmt.Errorf("the reason is too long (max %d characters)", maxReasonLen)
	}
	return blockoutInput{Start: start, End: end, Reason: reason}, nil
}

func monthURL(t time.Time) string {
	return "/calendar?year=" + t.Format("2006") + "&month=" + t.Format("1")
}

// ---- reading --------------------------------------------------------------

const blockoutSelect = `
	SELECT b.id::text, b.user_id::text, u.name, b.starts_on, b.ends_on, b.reason,
	       CASE WHEN b.created_by IS DISTINCT FROM b.user_id
	            THEN COALESCE(c.name, '') ELSE '' END
	FROM user_blockouts b
	JOIN users u ON u.id = b.user_id
	LEFT JOIN users c ON c.id = b.created_by
`

func scanBlockout(row pgx.Row, v viewer) (BlockoutRow, error) {
	var b BlockoutRow
	if err := row.Scan(&b.ID, &b.UserID, &b.UserName, &b.Starts, &b.Ends, &b.Reason, &b.AddedBy); err != nil {
		return b, err
	}
	b.Mine = b.UserID == v.ID
	b.CanEdit = canEditBlockout(v.Role, v.ID, b.UserID)
	return b, nil
}

// blockoutsInRange returns only what this viewer is allowed to see:
// everything for admins/managers, otherwise just their own rows.
// The filter is in SQL, so hidden rows never reach the page.
func (h *Handler) blockoutsInRange(ctx context.Context, v viewer, from, to time.Time) ([]BlockoutRow, error) {
	rows, err := h.db.Query(ctx, blockoutSelect+`
		WHERE b.starts_on < $2::date AND b.ends_on >= $1::date
		  AND ($3 OR b.user_id = $4)
		ORDER BY b.starts_on, lower(u.name)
	`, dateKey(from), dateKey(to), v.seesAll(), v.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BlockoutRow
	for rows.Next() {
		b, err := scanBlockout(rows, v)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// blockoutsByDay spreads each blockout over the days it covers, clamped to the
// visible grid so a very long blockout stays cheap.
func blockoutsByDay(rows []BlockoutRow, from, lastDay time.Time) map[string][]BlockoutRow {
	out := make(map[string][]BlockoutRow)
	for _, b := range rows {
		for _, d := range daysInRange(b.Starts, b.Ends, from, lastDay) {
			k := dateKey(d)
			out[k] = append(out[k], b)
		}
	}
	return out
}

// overlapsRange reports whether [start, end] touches [from, toExclusive).
func overlapsRange(start, end, from, toExclusive time.Time) bool {
	s, e, f, t := civil(start), civil(end), civil(from), civil(toExclusive)
	return s.Before(t) && !e.Before(f)
}

// ---- handlers -------------------------------------------------------------

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
	in, err := parseBlockoutInput(r.PostFormValue("starts_on"), r.PostFormValue("ends_on"), r.PostFormValue("reason"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// The person is only honoured when the policy allows it; otherwise the
	// logged-in user is always the owner, whatever the form says.
	target := u.ID
	if tid := strings.TrimSpace(r.PostFormValue("user_id")); tid != "" && tid != u.ID {
		if !canAddBlockoutFor(u.Role, u.ID, tid) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if !uuidRe.MatchString(tid) {
			http.Error(w, "unknown person", http.StatusBadRequest)
			return
		}
		target = tid
	}

	// INSERT ... SELECT FROM users: inserts nothing when the person does not exist.
	tag, err := h.db.Exec(r.Context(), `
		INSERT INTO user_blockouts (user_id, starts_on, ends_on, reason, created_by)
		SELECT id, $2::date, $3::date, $4::text, $5::uuid FROM users WHERE id = $1
	`, target, in.Start, in.End, in.Reason, u.ID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "unknown person", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, monthURL(in.Start), http.StatusSeeOther)
}

func (h *Handler) EditBlockoutForm(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Sign in required", http.StatusUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	if !uuidRe.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	b, err := scanBlockout(h.db.QueryRow(r.Context(), blockoutSelect+` WHERE b.id = $1`, id),
		viewer{ID: u.ID, Role: u.Role})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !b.CanEdit) {
		http.NotFound(w, r) // same answer for "missing" and "not yours": no existence leak
		return
	}
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	h.renderer.Render(w, r, http.StatusOK, "calendar_edit", map[string]any{
		"ID":          b.ID,
		"UserName":    b.UserName,
		"Mine":        b.Mine,
		"AddedBy":     b.AddedBy,
		"Reason":      b.Reason,
		"StartsValue": b.Starts.Format("2006-01-02"),
		"EndsValue":   b.Ends.Format("2006-01-02"),
		"BackURL":     monthURL(b.Starts),
	})
}

func (h *Handler) EditBlockout(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Sign in required", http.StatusUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	if !uuidRe.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad form", http.StatusBadRequest)
		return
	}
	in, err := parseBlockoutInput(r.PostFormValue("starts_on"), r.PostFormValue("ends_on"), r.PostFormValue("reason"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// One atomic statement: the WHERE clause IS the permission check
	// (same rule as canEditBlockout: manage-all OR owner).
	tag, err := h.db.Exec(r.Context(), `
		UPDATE user_blockouts
		SET starts_on = $2::date, ends_on = $3::date, reason = $4::text,
		    updated_at = now(), updated_by = $5::uuid
		WHERE id = $1 AND ($6 OR user_id = $5::uuid)
	`, id, in.Start, in.End, in.Reason, u.ID, canManageAllBlockouts(u.Role))
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, monthURL(in.Start), http.StatusSeeOther)
}

func (h *Handler) DeleteBlockout(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Sign in required", http.StatusUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	if !uuidRe.MatchString(id) {
		http.NotFound(w, r)
		return
	}

	tag, err := h.db.Exec(r.Context(), `
		DELETE FROM user_blockouts
		WHERE id = $1 AND ($2 OR user_id = $3::uuid)
	`, id, canManageAllBlockouts(u.Role), u.ID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		http.NotFound(w, r)
		return
	}

	// Return to the month the person was looking at.
	dest := "/calendar"
	if r.PostFormValue("year") != "" {
		if y, m, err := requestedMonth(r.PostFormValue("year"), r.PostFormValue("month"), time.Now()); err == nil {
			dest = fmt.Sprintf("/calendar?year=%d&month=%d", y, m)
		}
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}
