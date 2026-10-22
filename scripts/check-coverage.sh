#!/usr/bin/env bash
# Fails if any core package is below the coverage floor (default 80%).
# Usage: scripts/check-coverage.sh [floor]
set -euo pipefail
FLOOR="${1:-80}"
CORE=(checks alert alert/notify config reference feeds redact chain engine)
fail=0
for p in "${CORE[@]}"; do
  pct=$(go test -tags integration -cover "./internal/$p" | sed -nE 's/.*coverage: ([0-9.]+)%.*/\1/p')
  ok=$(awk -v a="$pct" -v b="$FLOOR" 'BEGIN { print (a >= b) ? "ok" : "LOW" }')
  printf '%-14s %6s%%  %s\n' "$p" "$pct" "$ok"
  [[ "$ok" == ok ]] || fail=1
done
exit $fail
