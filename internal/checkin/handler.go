package checkin

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourchurch/churchplanner/internal/platform/httpx"
)

type Handler struct {
	db       *pgxpool.Pool
	renderer *httpx.Renderer
}

type Child struct {
	ID        string
	FirstName string
	LastName  string
	CheckedIn bool
}

// generateCode returns a random string like "X7F-382"
func generateCode() string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const numbers = "0123456789"

	l1, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
	l2, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
	l3, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
	n1, _ := rand.Int(rand.Reader, big.NewInt(int64(len(numbers))))
	n2, _ := rand.Int(rand.Reader, big.NewInt(int64(len(numbers))))
	n3, _ := rand.Int(rand.Reader, big.NewInt(int64(len(numbers))))

	return fmt.Sprintf("%c%c%c-%c%c%c",
		letters[l1.Int64()], letters[l2.Int64()], letters[l3.Int64()],
		numbers[n1.Int64()], numbers[n2.Int64()], numbers[n3.Int64()])
}

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	phone := strings.TrimSpace(r.URL.Query().Get("phone"))

	if phone == "" {
		h.renderer.Render(w, r, http.StatusOK, "checkin_index", nil)
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT id, first_name, last_name FROM children WHERE parent_phone = $1 ORDER BY first_name
	`, phone)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	defer rows.Close()

	var children []Child
	for rows.Next() {
		var c Child
		if err := rows.Scan(&c.ID, &c.FirstName, &c.LastName); err != nil {
			httpx.ServerError(w, err)
			return
		}
		children = append(children, c)
	}

	h.renderer.Render(w, r, http.StatusOK, "checkin_family", map[string]any{
		"Phone":    phone,
		"Children": children,
	})
}

func (h *Handler) AddChildForm(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	h.renderer.Render(w, r, http.StatusOK, "checkin_add", map[string]any{"Phone": phone})
}

func (h *Handler) AddChild(w http.ResponseWriter, r *http.Request) {
	phone := r.FormValue("phone")
	first := strings.TrimSpace(r.FormValue("first_name"))
	last := strings.TrimSpace(r.FormValue("last_name"))

	_, err := h.db.Exec(r.Context(), `
		INSERT INTO children (parent_phone, first_name, last_name) VALUES ($1, $2, $3)
	`, phone, first, last)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	http.Redirect(w, r, "/checkin?phone="+phone, http.StatusSeeOther)
}

func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpx.ServerError(w, err)
		return
	}

	childIDs := r.Form["child_ids"]
	if len(childIDs) == 0 {
		http.Redirect(w, r, "/checkin", http.StatusSeeOther)
		return
	}

	code := generateCode()

	// In a real app, we'd use a transaction. Here, a loop is fine for a small church.
	for _, cid := range childIDs {
		_, err := h.db.Exec(r.Context(), `
			INSERT INTO checkins (child_id, security_code) VALUES ($1, $2)
		`, cid, code)
		if err != nil {
			httpx.ServerError(w, err)
			return
		}
	}

	// Fetch the children for the success tag
	rows, err := h.db.Query(r.Context(), `
		SELECT first_name, last_name FROM children WHERE id = ANY($1::uuid[])
	`, childIDs)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	defer rows.Close()

	var kids []Child
	for rows.Next() {
		var c Child
		rows.Scan(&c.ID, &c.FirstName, &c.LastName)
		kids = append(kids, c)
	}

	h.renderer.Render(w, r, http.StatusOK, "checkin_success", map[string]any{
		"Code":  code,
		"Kids":  kids,
		"Now":   time.Now(),
	})
}
