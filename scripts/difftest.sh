#!/usr/bin/env bash
# Differential test: run the C pgn-extract and pgnx on the same input, diff output.
#
# Usage: scripts/difftest.sh <input.pgn> [format]
#
# Environment:
#   C_BIN       C pgn-extract binary (default ~/code/pgn-extract/build/bin/pgn-extract)
#   GO_BIN      pgnx binary (default ./pgnx, built if missing)
#   PGNX_CMD    pgnx subcommand to run (default convert)
#   C_FLAGS     extra arguments for the C binary, verbatim (e.g. '-TpFischer')
#   PGNX_FLAGS  extra arguments for pgnx, verbatim (e.g. '--player Fischer')
#
# Example filter run:
#   PGNX_CMD=filter C_FLAGS='-TpFischer' PGNX_FLAGS='--player Fischer' scripts/difftest.sh big.pgn
set -euo pipefail
C_BIN="${C_BIN:-$HOME/code/pgn-extract/build/bin/pgn-extract}"
GO_BIN="${GO_BIN:-./pgnx}"
PGNX_CMD="${PGNX_CMD:-convert}"
C_FLAGS="${C_FLAGS:-}"
PGNX_FLAGS="${PGNX_FLAGS:-}"
FMT="${2:-san}"
IN="$1"

[ -x "$C_BIN" ] || { echo "C binary not found: $C_BIN (build with: make -C ~/code/pgn-extract)" >&2; exit 1; }
[ -x "$GO_BIN" ] || go build -o "$GO_BIN" ./cmd/pgnx

c_out=$(mktemp)
go_out=$(mktemp)
trap 'rm -f "$c_out" "$go_out"' EXIT

# shellcheck disable=SC2086  # C_FLAGS/PGNX_FLAGS are deliberately word-split.
case "$FMT" in
  san) "$C_BIN" $C_FLAGS --quiet -o"$c_out" "$IN" ;;
  *)   "$C_BIN" $C_FLAGS -W"$FMT" --quiet -o"$c_out" "$IN" ;;
esac
# pgnx exits 2 when some games were rejected (the C tool exits 0 for the
# same input); that is not a differential failure by itself.
"$GO_BIN" "$PGNX_CMD" $PGNX_FLAGS --quiet -f "$FMT" -o "$go_out" "$IN" || [ $? -eq 2 ]

if diff -u "$c_out" "$go_out"; then
  echo "OK: outputs identical for $IN ($FMT)"
else
  echo "DIVERGENCE for $IN ($FMT) — triage against CONFORMANCE.md" >&2
  exit 1
fi
