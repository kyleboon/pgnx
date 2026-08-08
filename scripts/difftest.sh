#!/usr/bin/env bash
# Differential test: run the C pgn-extract and pgnx on the same input, diff output.
# Usage: scripts/difftest.sh <input.pgn> [format]
set -euo pipefail
C_BIN="${C_BIN:-$HOME/code/pgn-extract/build/bin/pgn-extract}"
GO_BIN="${GO_BIN:-./pgnx}"
FMT="${2:-san}"
IN="$1"

[ -x "$C_BIN" ] || { echo "C binary not found: $C_BIN (build with: make -C ~/code/pgn-extract)" >&2; exit 1; }
[ -x "$GO_BIN" ] || go build -o "$GO_BIN" ./cmd/pgnx

c_out=$(mktemp)
go_out=$(mktemp)
trap 'rm -f "$c_out" "$go_out"' EXIT

case "$FMT" in
  san) "$C_BIN" --quiet -o"$c_out" "$IN" ;;
  *)   "$C_BIN" -W"$FMT" --quiet -o"$c_out" "$IN" ;;
esac
"$GO_BIN" convert --quiet -f "$FMT" -o "$go_out" "$IN"

if diff -u "$c_out" "$go_out"; then
  echo "OK: outputs identical for $IN ($FMT)"
else
  echo "DIVERGENCE for $IN ($FMT) — triage against CONFORMANCE.md" >&2
  exit 1
fi
