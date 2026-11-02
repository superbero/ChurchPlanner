#!/usr/bin/env bash
set -uo pipefail

[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  Diagnostic (before fixing)"
echo "════════════════════════════════════════════════════════════════"
echo "▸ All CreateTime call sites in handler.go:"
grep -n 'h\.plans\.CreateTime' internal/scheduling/handler.go || echo "  (none found)"
echo ""
echo "▸ Current CreateTime signature in plans.go:"
grep -n 'func (r \*PlanRepository) CreateTime' internal/scheduling/plans.go || echo "  (not found)"
echo ""
echo "▸ Lines 238-250 of handler.go (context around the error):"
sed -n '238,250p' internal/scheduling/handler.go
echo ""

echo "════════════════════════════════════════════════════════════════"
echo "  FIX 1/3 — CreateTime returns (PlanTime, error)"
echo "════════════════════════════════════════════════════════════════"
python3 - <<'PYEOF'
from pathlib import Path
p = Path("internal/scheduling/plans.go")
src = p.read_text()

start = src.find('func (r *PlanRepository) CreateTime(')
if start == -1:
    print("✗ CreateTime not found")
    raise SystemExit(1)

# Walk to matching closing brace
i = start
depth = 0
in_func = False
while i < len(src):
    if src[i] == '{':
        depth += 1
        in_func = True
    elif src[i] == '}':
        depth -= 1
        if in_func and depth == 0:
            i += 1
            break
    i += 1

new_func = '''func (r *PlanRepository) CreateTime(ctx context.Context, planID, kind, name string, starts, ends time.Time) (PlanTime, error) {
	var t PlanTime
	err := r.db.QueryRow(ctx, `
		INSERT INTO plan_times (plan_id, kind, name, starts_at, ends_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, plan_id, kind, name, starts_at, ends_at
	`, planID, kind, name, starts, ends).Scan(&t.ID, &t.PlanID, &t.Kind, &t.Name, &t.StartsAt, &t.EndsAt)
	return t, err
}'''

src = src[:start] + new_func + src[i:]
p.write_text(src)
print("✓ CreateTime now returns (PlanTime, error)")
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  FIX 2/3 — Update call sites that used 1-value form"
echo "════════════════════════════════════════════════════════════════"
python3 - <<'PYEOF'
from pathlib import Path
import re
p = Path("internal/scheduling/handler.go")
src = p.read_text()

changes = 0

# A) `if err := h.plans.CreateTime(...)` → `if _, err := h.plans.CreateTime(...)`
pattern_a = r'if err := (h\.plans\.CreateTime\()'
if re.search(pattern_a, src):
    src = re.sub(pattern_a, r'if _, err := \1', src)
    n = len(re.findall(pattern_a, src))
    changes += 1
    print(f"✓ fixed 'if err := h.plans.CreateTime' pattern (discard result)")

# B) `_ = h.plans.CreateTime(...)` → `_, _ = h.plans.CreateTime(...)`
#    Also handle `_ = h.plans.CreateTime` at start of line
if '_ = h.plans.CreateTime(' in src:
    src = src.replace('_ = h.plans.CreateTime(', '_, _ = h.plans.CreateTime(')
    changes += 1
    print("✓ fixed '_ = h.plans.CreateTime' pattern (discard both)")

# C) `_, err := h.plans.CreateTime(...)` — works as-is with 2 returns, no change needed
if re.search(r'_,\s*err\s*:=\s*h\.plans\.CreateTime', src):
    print("(_, err := pattern already correct for 2-return)")

if changes == 0:
    print("(no call sites needed 1-var → 2-var conversion)")

p.write_text(src)
PYEOF

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  FIX 3/3 — Build"
echo "════════════════════════════════════════════════════════════════"
if ! go build ./... 2>&1; then
    echo ""
    echo "✗ Build failed. Here's what CreateTime looks like now:"
    grep -A8 'func (r \*PlanRepository) CreateTime' internal/scheduling/plans.go
    echo ""
    echo "And here are all call sites:"
    grep -n 'h\.plans\.CreateTime' internal/scheduling/handler.go
    echo ""
    echo "Paste those two blocks so I can target the remaining mismatch."
    exit 1
fi
echo "✓ build OK"

echo ""
echo "▸ Restart"
docker compose -f deploy/docker-compose.yml up -d --build
echo ""
echo "▸ Logs (20s)"
timeout 20 docker compose -f deploy/docker-compose.yml logs -f app 2>&1 || true

echo ""
echo "Look for:  {\"level\":\"INFO\",\"msg\":\"listening\",\"addr\":\":8080\"}"
echo "Then HARD-REFRESH and re-run the 5 plan-form checks."
