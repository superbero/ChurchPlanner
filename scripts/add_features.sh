#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  Fixing people/handler.go Update function"
echo "════════════════════════════════════════════════════════════════"

python3 - <<'PYEOF'
from pathlib import Path
import re
import sys

p = Path("internal/people/handler.go")
src = p.read_text()

# Find the start of the Update function
start_marker = 'func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {'
start_idx = src.find(start_marker)
if start_idx == -1:
    print("✗ Could not find 'func (h *Handler) Update' in people/handler.go")
    sys.exit(1)

# Walk braces to find the end of the function
brace_count = 0
in_func = False
end_idx = start_idx

for i in range(start_idx, len(src)):
    if src[i] == '{':
        brace_count += 1
        in_func = True
    elif src[i] == '}':
        brace_count -= 1
        if in_func and brace_count == 0:
            end_idx = i + 1
            break

if not in_func or brace_count != 0:
    print("✗ Could not find the end of the Update function (unbalanced braces)")
    sys.exit(1)

new_func = '''func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Combine country code and local number
	phoneCountry := strings.TrimSpace(r.FormValue("phone_country"))
	phoneLocal := strings.TrimSpace(r.FormValue("phone_number"))
	phone := phoneLocal
	if phoneCountry != "" && !strings.HasPrefix(phoneLocal, phoneCountry) {
		phone = phoneCountry + " " + phoneLocal
	}

	userRole := strings.TrimSpace(r.FormValue("role"))

	// Pass empty strings for vocal ranges since the UI no longer captures them
	if err := h.repo.Update(r.Context(), id, phone, "", "", userRole); err != nil {
		httpx.ServerError(w, err)
		return
	}

	positionIDs := r.Form["position_ids"]
	if err := h.repo.SetPositions(r.Context(), id, positionIDs); err != nil {
		httpx.ServerError(w, err)
		return
	}

	memberships := map[string]string{}
	for key, values := range r.Form {
		if !strings.HasPrefix(key, "team_role_") || len(values) == 0 {
			continue
		}
		teamRole := strings.TrimSpace(values[0])
		if teamRole == "" {
			continue
		}
		teamID := strings.TrimPrefix(key, "team_role_")
		memberships[teamID] = teamRole
	}
	if err := h.memberships.SetForUser(r.Context(), id, memberships); err != nil {
		httpx.ServerError(w, err)
		return
	}

	http.Redirect(w, r, "/people/"+id, http.StatusSeeOther)
}'''

# Replace the old function with the new one
new_src = src[:start_idx] + new_func + src[end_idx:]
p.write_text(new_src)
print("  ✓ Update function completely replaced with correct phone/role logic")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  Build & Restart"
echo "════════════════════════════════════════════════════════════════"
if ! go build ./...; then
    echo "✗ build failed — paste errors"
    exit 1
fi
echo "✓ build OK"

docker compose -f deploy/docker-compose.yml up -d --build
