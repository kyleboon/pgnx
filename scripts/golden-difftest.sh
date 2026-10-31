#!/usr/bin/env bash
# Run scripts/difftest.sh over every golden input (plain `convert`, SAN) and
# fail on any divergence that is not listed in golden-difftest-known.txt.
#
# Usage: scripts/golden-difftest.sh [out-dir]
#   C_BIN   C pgn-extract binary (see scripts/difftest.sh)
#
# Diffs for divergent files land in out-dir (default difftest-out/).
set -uo pipefail
cd "$(dirname "$0")/.."
OUT="${1:-difftest-out}"
KNOWN=scripts/golden-difftest-known.txt
mkdir -p "$OUT"
go build -o pgnx ./cmd/pgnx || exit 1

pass=0 known=0 unexpected=0 fixed=0
for f in cmd/pgnx/testdata/golden/infiles/*.pgn; do
  name=$(basename "$f")
  if grep -qx "$name" "$KNOWN"; then expected=1; else expected=0; fi
  if GO_BIN=./pgnx scripts/difftest.sh "$f" >"$OUT/$name.diff" 2>&1; then
    rm -f "$OUT/$name.diff"
    if [ $expected = 1 ]; then
      echo "FIXED?    $name matches the C tool but is listed in $KNOWN"
      fixed=$((fixed + 1))
    else
      pass=$((pass + 1))
    fi
  elif [ $expected = 1 ]; then
    echo "known     $name (documented in CONFORMANCE.md)"
    known=$((known + 1))
  else
    echo "DIVERGED  $name  -> $OUT/$name.diff"
    unexpected=$((unexpected + 1))
  fi
done

echo
echo "identical: $pass   known divergences: $known   unexpected: $unexpected   stale known entries: $fixed"
[ "$unexpected" = 0 ] && [ "$fixed" = 0 ]
