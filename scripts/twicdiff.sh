#!/usr/bin/env bash
# Corpus differential: run the C pgn-extract and pgnx over every PGN file in
# a directory (one process per file, in parallel), compare byte-for-byte and
# keep a unified diff for each file that diverges.
#
# Usage: scripts/twicdiff.sh [pgn-dir] [out-dir]
#   pgn-dir   directory of *.pgn inputs (default twic/twic/pgn)
#   out-dir   where results go (default twic/diff); it is wiped first
#
# Environment:
#   C_BIN    C pgn-extract binary (default ~/code/pgn-extract/build/bin/pgn-extract)
#   GO_BIN   pgnx binary (default ./pgnx, built if missing)
#   FMT      output format (default san)
#   JOBS     parallel workers (default 8)
#
# Output (in out-dir):
#   results.tsv        one row per input: status (OK|DIFF) <tab> file <tab>
#                      rejected games <tab> warnings (pgnx stderr counts)
#   <file>.diff        unified diff (C vs pgnx) for each divergent input
#   <file>.err         pgnx stderr for each input that logged anything
#
# The run exits 1 if any input diverged. Use LC_ALL=C when grepping the
# diffs: the corpus contains raw Latin-1 bytes that break UTF-8 tools.
set -euo pipefail

C_BIN="${C_BIN:-$HOME/code/pgn-extract/build/bin/pgn-extract}"
GO_BIN="${GO_BIN:-./pgnx}"
FMT="${FMT:-san}"
JOBS="${JOBS:-8}"

# Worker mode: scripts/twicdiff.sh --worker <out-dir> <input.pgn>
if [ "${1:-}" = "--worker" ]; then
  out="$2"; in="$3"; name=$(basename "$in" .pgn)
  c_out="$out/$name.c"; go_out="$out/$name.go"; err="$out/$name.err"
  # shellcheck disable=SC2086
  case "$FMT" in
    san) "$C_BIN" --quiet -o"$c_out" "$in" 2>/dev/null ;;
    *)   "$C_BIN" -W"$FMT" --quiet -o"$c_out" "$in" 2>/dev/null ;;
  esac
  # pgnx exits 2 when some games were rejected; not a differential failure.
  "$GO_BIN" convert --quiet -f "$FMT" -o "$go_out" "$in" 2>"$err" || [ $? -eq 2 ]
  rejected=$(LC_ALL=C grep -Ec ': game [0-9]+: ' "$err" | tr -d ' ' || true)
  warnings=$(LC_ALL=C grep -Ec ': game [0-9]+: warning: ' "$err" | tr -d ' ' || true)
  rejected=$(( ${rejected:-0} - ${warnings:-0} ))
  [ -s "$err" ] || rm -f "$err"
  if cmp -s "$c_out" "$go_out"; then
    status=OK
  else
    status=DIFF
    LC_ALL=C diff -u "$c_out" "$go_out" > "$out/$name.diff" || true
  fi
  rm -f "$c_out" "$go_out"
  printf '%s\t%s\t%s\t%s\n' "$status" "$name" "$rejected" "${warnings:-0}" >> "$out/results.tsv"
  exit 0
fi

PGN_DIR="${1:-twic/twic/pgn}"
OUT_DIR="${2:-twic/diff}"

[ -x "$C_BIN" ] || { echo "C binary not found: $C_BIN (build with: make -C ~/code/pgn-extract)" >&2; exit 1; }
[ -x "$GO_BIN" ] || go build -o "$GO_BIN" ./cmd/pgnx
[ -d "$PGN_DIR" ] || { echo "input directory not found: $PGN_DIR" >&2; exit 1; }

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"
export C_BIN GO_BIN FMT

start=$(date +%s)
find "$PGN_DIR" -maxdepth 1 -name '*.pgn' -print0 | sort -z \
  | xargs -0 -n1 -P "$JOBS" "$0" --worker "$OUT_DIR"

sort -k2,2V -o "$OUT_DIR/results.tsv" "$OUT_DIR/results.tsv"
total=$(wc -l < "$OUT_DIR/results.tsv" | tr -d ' ')
ok=$(awk -F'\t' '$1=="OK"' "$OUT_DIR/results.tsv" | wc -l | tr -d ' ')
diffs=$(awk -F'\t' '$1=="DIFF"' "$OUT_DIR/results.tsv" | wc -l | tr -d ' ')
rejected=$(awk -F'\t' '{s+=$3} END{print s+0}' "$OUT_DIR/results.tsv")
warnings=$(awk -F'\t' '{s+=$4} END{print s+0}' "$OUT_DIR/results.tsv")
echo "twicdiff ($FMT): $ok/$total identical, $diffs divergent; pgnx rejected $rejected games, $warnings warnings; $(( $(date +%s) - start ))s"
echo "results: $OUT_DIR/results.tsv"
[ "$diffs" -eq 0 ]
