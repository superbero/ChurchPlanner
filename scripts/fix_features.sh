#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  Fixing Team Leader Permissions (Media, Worship, Kids, Sound)"
echo "════════════════════════════════════════════════════════════════"

python3 <<'PYEOF'
from pathlib import Path
import sys

# ─────────────────────────────────────────────────────────────
# 1. FIX canManagePosition in handler.go
# Allow Admin, Editor, AND Team Leaders to manage their positions.
# ─────────────────────────────────────────────────────────────
p = Path("internal/scheduling/handler.go")
src = p.read_text()

old_perm = '''	if u.Role == auth.RoleAdmin {
		return true, nil
	}'''
new_perm = '''	if auth.CanEditContent(u.Role) {
		return true, nil
	}'''

if old_perm in src:
    src = src.replace(old_perm, new_perm, 1)
    p.write_text(src)
    print("✓ handler.go: canManagePosition now allows Editors and Team Leaders")
elif 'if auth.CanEditContent(u.Role) {' in src:
    print("· handler.go: canManagePosition already fixed")
else:
    print("! handler.go: pattern not found. Context:")
    idx = src.find("func (h *Handler) canManagePosition")
    if idx >= 0: print(src[idx:idx+300])
    sys.exit(1)

# ─────────────────────────────────────────────────────────────────
# 2. Ensure ViewPlan populates ManageablePositions for Team Leaders
# (This hides the "Assign" button for teams they don't lead).
# ─────────────────────────────────────────────────────────────────
old_view = '''	// Compute which positions the current user may edit.
	var manageableIDs = map[string]bool{}
	var manageablePositions []Position
	if u, ok := auth.UserFrom(r.Context()); ok {
		if u.Role == "admin" {
			for _, pos := range allPositions {
				manageableIDs[pos.ID] = true
			}
			manageablePositions = allPositions
		} else {
			leaderTeams, _ := h.members.LeaderTeams(r.Context(), u.ID)
			teamSet := map[string]bool{}
			for _, tid := range leaderTeams {
				teamSet[tid] = true
			}
			for _, pos := range allPositions {
				if teamSet[pos.TeamID] {
					manageableIDs[pos.ID] = true
					manageablePositions = append(manageablePositions, pos)
				}
			}
		}
	}'''

new_view = '''	// Compute which positions the current user may edit.
	var manageableIDs = map[string]bool{}
	var manageablePositions []Position
	if u, ok := auth.UserFrom(r.Context()); ok {
		if auth.CanEditContent(u.Role) {
			for _, pos := range allPositions {
				manageableIDs[pos.ID] = true
			}
			manageablePositions = allPositions
		} else {
			// It's a Team Leader. Fetch the teams they lead.
			leaderTeams, _ := h.members.LeaderTeams(r.Context(), u.ID)
			teamSet := map[string]bool{}
			for _, tid := range leaderTeams {
				teamSet[tid] = true
			}
			for _, pos := range allPositions {
				if teamSet[pos.TeamID] {
					manageableIDs[pos.ID] = true
					manageablePositions = append(manageablePositions, pos)
				}
			}
		}
	}'''

if old_view in src:
    src = src.replace(old_view, new_view, 1)
    p.write_text(src)
    print("✓ handler.go: ViewPlan now filters positions based on Team Leader status")
elif 'manageableIDs[pos.ID] = true' in src:
    print("· handler.go: ViewPlan already filters positions")
else:
    print("· handler.go: ViewPlan structure differs; skipping (UI might show all positions, but server will block unauthorized edits).")

# ─────────────────────────────────────────────────────────────
# 3. Ensure routes.go has the nil-pointer fix
# ─────────────────────────────────────────────────────────────
p = Path("internal/scheduling/routes.go")
src = p.read_text()
if "members:" not in src:
    import re
    src = re.sub(
        r'(assign:\s*NewAssignmentRepository\(db\),\s*)',
        r'\1\n\t\tmembers: NewTeamMembershipRepository(db),',
        src
    )
    p.write_text(src)
    print("✓ routes.go: Injected members repository (nil-pointer fixed)")
else:
    print("· routes.go: members already present")

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

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  HOW TO TEST THE TEAM LEADER WORKFLOW"
echo "════════════════════════════════════════════════════════════════"
cat <<'CHECKLIST'
1. Log in as Admin → /people.
2. Open "Michael Smith".
   → Set Role to "Volunteer".
   → Under Team Memberships: Set "Media Team" to "Leader".
   → Set "Worship Team" to "— not a member —".
   → Save Profile.

3. Log out, log in as Michael (Volunteer + Media Leader).
   → He lands on /plans.
   → Open a plan.

4. Look at the "Add a needed position" dropdown.
   → It ONLY shows Media Team positions (Sound, Lights, Livestream).
   → It does NOT show Worship or Kids.

5. Look at the "Worship Leader" row in the People Serving table.
   → The "Assign..." dropdown should be hidden (shows "—").

6. Look at the "Sound" row.
   → The "Assign..." dropdown works. Assign someone.

7. Log in as Admin. Open the same plan.
   → The Admin sees ALL positions and can edit everything.

This is exactly how Planning Center works.
CHECKLIST
