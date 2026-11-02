package checkin

import (
	"github.com/go-chi/chi/v5"
	"encoding/base64"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	qrcode "github.com/skip2/go-qrcode"

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

	var familyID string
	err := h.db.QueryRow(r.Context(), `SELECT family_id FROM guardians WHERE phone = $1 LIMIT 1`, phone).Scan(&familyID)
	if err != nil {
		// Family not found for this number
		h.renderer.Render(w, r, http.StatusOK, "checkin_family", map[string]any{
			"Phone": phone, "FamilyID": "", "Children": nil,
		})
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT id, first_name, last_name FROM children WHERE family_id = $1 ORDER BY first_name
	`, familyID)
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
		"FamilyID": familyID,
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

	var familyID string
	err := h.db.QueryRow(r.Context(), `SELECT family_id FROM guardians WHERE phone = $1 LIMIT 1`, phone).Scan(&familyID)
	if err != nil {
		// Create new family and guardian
		err = h.db.QueryRow(r.Context(), `INSERT INTO families DEFAULT VALUES RETURNING id`).Scan(&familyID)
		if err == nil {
			h.db.Exec(r.Context(), `INSERT INTO guardians (family_id, phone) VALUES ($1, $2)`, familyID, phone)
		}
	}

	_, err = h.db.Exec(r.Context(), `
		INSERT INTO children (family_id, first_name, last_name) VALUES ($1, $2, $3)
	`, familyID, first, last)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	http.Redirect(w, r, "/checkin?phone="+phone, http.StatusSeeOther)
}

func makeQRCode(code string, kids []Child) string {
	qrData := fmt.Sprintf("Code: %s\nKids: ", code)
	for _, k := range kids {
		qrData += k.FirstName + " " + k.LastName + ", "
	}
	qrBytes, err := qrcode.Encode(qrData, qrcode.Medium, 256)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(qrBytes)
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

	// Generate a QR code that staff can scan to verify pickup.
	qrBase64 := makeQRCode(code, kids)

	h.renderer.Render(w, r, http.StatusOK, "checkin_success", map[string]any{
		"Code":   code,
		"Kids":   kids,
		"Now":    time.Now(),
		"QRCode": qrBase64,
	})
}


// ManageFamily shows all kids and guardians associated with a phone number.
func (h *Handler) ManageFamily(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")

	var familyID string
	err := h.db.QueryRow(r.Context(), `SELECT family_id FROM guardians WHERE phone = $1 LIMIT 1`, phone).Scan(&familyID)
	if err != nil {
		h.renderer.Render(w, r, http.StatusOK, "checkin_manage", map[string]any{
			"Phone": phone, "FamilyID": "", "Children": nil, "Guardians": nil,
		})
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT id, first_name, last_name FROM children WHERE family_id = $1 ORDER BY first_name
	`, familyID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	var children []Child
	for rows.Next() {
		var c Child
		rows.Scan(&c.ID, &c.FirstName, &c.LastName)
		children = append(children, c)
	}
	rows.Close()

	grows, err := h.db.Query(r.Context(), `
		SELECT id, name, phone FROM guardians WHERE family_id = $1 ORDER BY name
	`, familyID)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	type Guardian struct { ID, Name, Phone string }
	var guardians []Guardian
	for grows.Next() {
		var g Guardian
		grows.Scan(&g.ID, &g.Name, &g.Phone)
		guardians = append(guardians, g)
	}
	grows.Close()

	h.renderer.Render(w, r, http.StatusOK, "checkin_manage", map[string]any{
		"Phone":     phone,
		"FamilyID":  familyID,
		"Children":  children,
		"Guardians": guardians,
	})
}

// AddGuardian links another parent/spouse to the family.
func (h *Handler) AddGuardian(w http.ResponseWriter, r *http.Request) {
	familyID := r.FormValue("family_id")
	phone := strings.TrimSpace(r.FormValue("phone"))
	name := strings.TrimSpace(r.FormValue("name"))
	origPhone := r.FormValue("orig_phone")

	if familyID != "" && phone != "" {
		h.db.Exec(r.Context(), `INSERT INTO guardians (family_id, name, phone) VALUES ($1, $2, $3)`, familyID, name, phone)
	}

	http.Redirect(w, r, "/checkin/family?phone="+origPhone, http.StatusSeeOther)
}

func (h *Handler) DeleteGuardian(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	origPhone := r.FormValue("orig_phone")
	h.db.Exec(r.Context(), `DELETE FROM guardians WHERE id = $1`, id)
	http.Redirect(w, r, "/checkin/family?phone="+origPhone, http.StatusSeeOther)
}

func (h *Handler) EditChildForm(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var c Child
	err := h.db.QueryRow(r.Context(), `SELECT id, first_name, last_name FROM children WHERE id = $1`, id).Scan(&c.ID, &c.FirstName, &c.LastName)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// We need the phone to pass through for redirects
	var phone string
	_ = h.db.QueryRow(r.Context(), `SELECT parent_phone FROM children WHERE id = $1`, id).Scan(&phone)

	h.renderer.Render(w, r, http.StatusOK, "checkin_edit", map[string]any{
		"Child": c,
		"Phone": phone,
	})
}

func (h *Handler) UpdateChild(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	first := strings.TrimSpace(r.FormValue("first_name"))
	last := strings.TrimSpace(r.FormValue("last_name"))
	phone := r.FormValue("phone")

	_, err := h.db.Exec(r.Context(), `UPDATE children SET first_name = $1, last_name = $2 WHERE id = $3`, first, last, id)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/checkin/family?phone="+phone, http.StatusSeeOther)
}

func (h *Handler) DeleteChild(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	phone := r.FormValue("phone")

	_, err := h.db.Exec(r.Context(), `DELETE FROM children WHERE id = $1`, id)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	http.Redirect(w, r, "/checkin/family?phone="+phone, http.StatusSeeOther)
}


type PickupPass struct {
	Code string
	Kids []Child
	QR   string
}

// RetrieveCode shows active pickup codes for a family.
func (h *Handler) RetrieveCode(w http.ResponseWriter, r *http.Request) {
	phone := strings.TrimSpace(r.URL.Query().Get("phone"))

	if phone == "" {
		http.Redirect(w, r, "/checkin", http.StatusSeeOther)
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT c.security_code, ch.id, ch.first_name, ch.last_name
		FROM checkins c
		JOIN children ch ON ch.id = c.child_id
		JOIN guardians g ON g.family_id = ch.family_id
		WHERE g.phone = $1 AND c.checked_out_at IS NULL
		ORDER BY c.checked_in_at DESC
	`, phone)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	defer rows.Close()

	// Group children by security code
	passesMap := make(map[string][]Child)
	var codes []string
	for rows.Next() {
		var code string
		var c Child
		if err := rows.Scan(&code, &c.ID, &c.FirstName, &c.LastName); err != nil {
			httpx.ServerError(w, err)
			return
		}
		if _, exists := passesMap[code]; !exists {
			codes = append(codes, code)
		}
		passesMap[code] = append(passesMap[code], c)
	}

	var passes []PickupPass
	for _, code := range codes {
		kids := passesMap[code]
		passes = append(passes, PickupPass{
			Code: code,
			Kids: kids,
			QR:   makeQRCode(code, kids),
		})
	}

	h.renderer.Render(w, r, http.StatusOK, "checkin_retrieve", map[string]any{
		"Phone":  phone,
		"Passes": passes,
	})
}
