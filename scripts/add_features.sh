#!/usr/bin/env bash
set -euo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  1/4  Refactor QR generation & add RetrieveCode handler"
echo "════════════════════════════════════════════════════════════════"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/checkin/handler.go")
src = p.read_text()

# 1. Refactor QR generation into a helper function
# Find the Submit function
start_submit = src.find('func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {')
if start_submit == -1:
    print("  ! Submit not found")
    raise SystemExit(1)

# Add the helper before Submit
helper = '''func makeQRCode(code string, kids []Child) string {
	qrData := fmt.Sprintf("Code: %s\\nKids: ", code)
	for _, k := range kids {
		qrData += k.FirstName + " " + k.LastName + ", "
	}
	qrBytes, err := qrcode.Encode(qrData, qrcode.Medium, 256)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(qrBytes)
}

'''
src = src[:start_submit] + helper + src[start_submit:]

# 2. Update Submit to use the helper
old_qr = '''	// Generate a QR code that staff can scan to verify pickup.
	qrData := fmt.Sprintf("Code: %s\\nKids: ", code)
	for _, k := range kids {
		qrData += k.FirstName + " " + k.LastName + ", "
	}

	qrBytes, err := qrcode.Encode(qrData, qrcode.Medium, 256)
	var qrBase64 string
	if err == nil {
		qrBase64 = base64.StdEncoding.EncodeToString(qrBytes)
	}

	h.renderer.Render(w, r, http.StatusOK, "checkin_success", map[string]any{
		"Code":   code,
		"Kids":   kids,
		"Now":    time.Now(),
		"QRCode": qrBase64,
	})'''
new_qr = '''	// Generate a QR code that staff can scan to verify pickup.
	qrBase64 := makeQRCode(code, kids)

	h.renderer.Render(w, r, http.StatusOK, "checkin_success", map[string]any{
		"Code":   code,
		"Kids":   kids,
		"Now":    time.Now(),
		"QRCode": qrBase64,
	})'''
if old_qr in src:
    src = src.replace(old_qr, new_qr, 1)

# 3. Add the RetrieveCode handler at the end
if "func (h *Handler) RetrieveCode" not in src:
    src += '''

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
'''
    p.write_text(src)
    print("  ✓ handler updated")
else:
    print("  · RetrieveCode already exists")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  2/4  Update Routes"
echo "════════════════════════════════════════════════════════════════"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/checkin/routes.go")
src = p.read_text()

if 'r.Get("/retrieve"' not in src:
    src = src.replace(
        'r.Get("/family", m.handler.ManageFamily)',
        'r.Get("/retrieve", m.handler.RetrieveCode)\n\t\tr.Get("/family", m.handler.ManageFamily)'
    )
    p.write_text(src)
    print("  ✓ route added")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  3/4  Templates: Retrieve page & Update Family page"
echo "════════════════════════════════════════════════════════════════"
cat > internal/checkin/templates/retrieve.html <<'HTMLEOF'
{{define "content"}}
<div class="kiosk-container" style="text-align: center;">
    <h1>Pickup Codes</h1>
    <p class="muted">Phone: {{.Phone}}</p>

    {{if .Passes}}
        <p class="muted" style="margin-bottom: 2rem;">Show these QR codes or codes to the check-in staff when picking up.</p>

        {{range .Passes}}
        <div class="pickup-pass">
            <div class="security-code">{{.Code}}</div>
            {{if .QR}}
            <div class="qr-container">
                <img src="data:image/png;base64,{{.QR}}" alt="QR Code" width="180" height="180">
            </div>
            {{end}}
            <div style="margin-top: 1rem; border-top: 1px dashed #ccc; padding-top: 1rem;">
                {{range .Kids}}
                <p class="tag-kid">{{.FirstName}} {{.LastName}}</p>
                {{end}}
            </div>
        </div>
        {{end}}
    {{else}}
        <div style="padding: 2rem; background: #fef0c7; border-radius: 8px; margin: 2rem 0;">
            <h2 style="margin-top: 0; color: #93370d;">No Active Check-ins</h2>
            <p>There are currently no children from this family checked in.</p>
        </div>
    {{end}}

    <p style="margin-top:3rem"><a href="/checkin?phone={{.Phone}}" class="button-secondary">← Back to Check-in</a></p>
</div>
{{end}}
HTMLEOF

python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/checkin/templates/family.html")
src = p.read_text()

# Add Retrieve button before the Cancel link
old_links = '''    <p style="margin-top:2rem">
        <a href="/checkin/family?phone={{.Phone}}">Manage Family / Add Child</a> ·
        <a href="/checkin">Cancel</a>
    </p>'''

new_links = '''    <p style="margin-top:2rem">
        {{if .Children}}
        <a href="/checkin/retrieve?phone={{.Phone}}" class="button-secondary" style="margin-right: 1rem;">🔍 Retrieve Pickup Code</a>
        {{end}}
        <a href="/checkin/family?phone={{.Phone}}">Manage Family / Add Child</a> ·
        <a href="/checkin">Cancel</a>
    </p>'''

if old_links in src:
    p.write_text(src.replace(old_links, new_links, 1))
    print("  ✓ family.html updated")
else:
    print("  ! family.html link block not found")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  4/4  CSS, Build & Restart"
echo "════════════════════════════════════════════════════════════════"
cat >> internal/web/static/css/app.css <<'CSSEOF'

/* Pickup Passes */
.pickup-pass {
    background: #f7f9fc;
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 2rem;
    margin-bottom: 2rem;
}
CSSEOF

if ! go build ./...; then
    echo "✗ build failed — paste errors"
    exit 1
fi
echo "✓ build OK"

docker compose -f deploy/docker-compose.yml up -d --build
echo ""
echo "✅ Done."
