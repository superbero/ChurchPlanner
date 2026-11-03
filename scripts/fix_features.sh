#!/usr/bin/env bash
set +e

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  What's ACTUALLY in the code (verbatim)"
echo "════════════════════════════════════════════════════════════════"

echo ""
echo "▸ auth helpers — all of them:"
echo "───────────────────────────────────────────────────────────────"
cat internal/platform/auth/middleware.go

echo ""
echo "▸ router.go — how /plans is gated:"
echo "───────────────────────────────────────────────────────────────"
grep -n -B3 -A8 'schedulingModule.Routes' internal/web/router.go

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  Decision: is the editor role actually blocked anywhere?"
echo "════════════════════════════════════════════════════════════════"

# Check 1: CanViewPlans returns true for editor?
if grep -q 'func CanViewPlans' internal/platform/auth/middleware.go; then
    echo "✓ auth.CanViewPlans exists"
else
    echo "✗ auth.CanViewPlans missing"
fi

# Check 2: canManagePosition uses CanEditContent?
if grep -q 'if auth.CanEditContent(u.Role) {' internal/scheduling/handler.go; then
    echo "✓ canManagePosition uses CanEditContent (editors can assign)"
else
    echo "✗ canManagePosition still admin-only"
fi

# Check 3: mutation handlers use CanEditContent?
COUNT=$(grep -c '!auth.CanEditContent(u.Role)' internal/scheduling/handler.go)
echo "✓ $COUNT mutation handlers use CanEditContent"

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  Build + restart"
echo "════════════════════════════════════════════════════════════════"
if ! go build ./...; then
    echo "✗ build failed — paste errors"
    exit 1
fi
echo "✓ build OK"

docker compose -f deploy/docker-compose.yml up -d --build

echo ""
echo "▸ Logs (10s)"
timeout 10 docker compose -f deploy/docker-compose.yml logs -f app 2>&1 || true

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  Behavioral test (this is the only test that matters)"
echo "════════════════════════════════════════════════════════════════"
cat <<'CHECKLIST'
  1. Log in as Jane (editor) in a fresh browser window.
     → If she lands on /plans: middleware + redirect both work.
     → If she lands on /my-schedule: middleware still blocks; paste the
       'router.go — how /plans is gated' output above.

  2. As Jane, on a plan:
     → Click "Assign" on a position.
     → If dropdown appears and assignment saves: canManagePosition fix works.
     → If you get 403: paste the auth/middleware.go output above.

  3. As Jane, /people → should 403.
     → If you can see the People page: auth.CanManagePeople is too permissive.
CHECKLIST
