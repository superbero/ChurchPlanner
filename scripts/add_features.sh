#!/usr/bin/env bash
[ -f go.mod ] || { echo "✗ run from project root"; exit 1; }

echo "════════════════════════════════════════════════════════════════"
echo "  DIAGNOSTIC: Lines 440-470 of handler.go (error is at 450)"
echo "════════════════════════════════════════════════════════════════"
sed -n '440,470p' internal/scheduling/handler.go

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  All function signatures in the file"
echo "════════════════════════════════════════════════════════════════"
grep -n "^func (h \*Handler)" internal/scheduling/handler.go

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  gofmt error report"
echo "════════════════════════════════════════════════════════════════"
gofmt -l -e internal/scheduling/handler.go 2>&1 | head -20
