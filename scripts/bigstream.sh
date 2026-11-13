#!/usr/bin/env bash
# Streaming check (ADR 0003): feed a multi-gigabyte PGN stream to pgnx and
# to the C pgn-extract on stdin and report wall time, throughput and peak
# RSS for both. Memory must stay bounded by the largest game, not by the
# input, and single-threaded throughput is compared with the C tool.
#
# Usage: scripts/bigstream.sh [size-gb] [pgn-dir]
#   size-gb   how much PGN to stream, in GB (default 2; decimals allowed,
#             e.g. 0.05 for a quick smoke test)
#   pgn-dir   directory of *.pgn files to concatenate, cycling through them
#             until the size is reached (default twic/twic/pgn)
#
# Environment:
#   C_BIN   C pgn-extract binary (default ~/code/pgn-extract/build/bin/pgn-extract)
#   GO_BIN  pgnx binary (default ./pgnx, built if missing)
#   FMT     output format (default san)
#
# Output goes to /dev/null; diagnostics are discarded. Not part of go test:
# it takes minutes (pgnx streams about 15 MB/s at the 2026-11-13 baseline,
# so 2 GB is about 2.5 minutes). The ADR 0003 baseline slice is the first
# 100 TWIC issues, 232,960,652 bytes: scripts/bigstream.sh 0.232960652
#   2026-11-02: C 19 s / 12 MB/s / 3 MB RSS;   pgnx 159 s / 1.4 MB/s / 15 MB RSS
#   2026-11-12: C 15.7 s / 14.8 MB/s / 2.8 MB RSS; pgnx 24.8 s / 9.4 MB/s / 13.4 MB RSS
#   2026-11-13: C 15.7 s / 14.8 MB/s / 2.9 MB RSS; pgnx 19.0 s / 12.3 MB/s / 13.5 MB RSS
#   2026-11-13: C 15.7 s / 14.8 MB/s / 2.9 MB RSS; pgnx 15.9 s / 14.7 MB/s / 13.7 MB RSS (perf.4)
set -euo pipefail

C_BIN="${C_BIN:-$HOME/code/pgn-extract/build/bin/pgn-extract}"
GO_BIN="${GO_BIN:-./pgnx}"
FMT="${FMT:-san}"
SIZE_GB="${1:-2}"
PGN_DIR="${2:-twic/twic/pgn}"

[ -x "$C_BIN" ] || { echo "C binary not found: $C_BIN (build with: make -C ~/code/pgn-extract)" >&2; exit 1; }
[ -x "$GO_BIN" ] || go build -o "$GO_BIN" ./cmd/pgnx
[ -d "$PGN_DIR" ] || { echo "input directory not found: $PGN_DIR" >&2; exit 1; }
command -v /usr/bin/time >/dev/null || { echo "/usr/bin/time not found" >&2; exit 1; }

# Pick the files to stream: the directory's *.pgn in name order, repeated
# until the requested size is reached.
target=$(awk -v gb="$SIZE_GB" 'BEGIN{printf "%d", gb * 1000000000}')
files=()
total=0
pool=()
while IFS= read -r f; do pool+=("$f"); done < <(find "$PGN_DIR" -maxdepth 1 -name '*.pgn' | sort)
[ "${#pool[@]}" -gt 0 ] || { echo "no *.pgn files in $PGN_DIR" >&2; exit 1; }
while [ "$total" -lt "$target" ]; do
  for f in "${pool[@]}"; do
    files+=("$f")
    total=$(( total + $(wc -c < "$f") ))
    [ "$total" -lt "$target" ] || break
  done
done
echo "streaming $(( total / 1000000 )) MB (${#files[@]} files from $PGN_DIR) to each tool"

stream() { cat "${files[@]}"; }

# now prints the time in seconds with sub-second precision where perl is
# available (macOS date has no %N).
now() {
  if command -v perl >/dev/null; then perl -MTime::HiRes=time -e 'printf "%.3f\n", time'; else date +%s; fi
}

# measure <label> <command...>: runs the command on the stream with its
# stdout and stderr discarded, under /usr/bin/time, and prints one line
# with wall time, throughput and peak RSS. The exit status is tolerated
# (pgnx exits 2 when some games were rejected).
measure() {
  local label="$1"; shift
  local tfile; tfile=$(mktemp)
  local start end secs rss_kb
  start=$(now)
  if [ "$(uname)" = "Darwin" ]; then
    stream | /usr/bin/time -l -o "$tfile" "$@" >/dev/null 2>&1 || true
    # "  12345678  maximum resident set size" (bytes)
    rss_kb=$(awk '/maximum resident set size/ {printf "%d", $1 / 1024}' "$tfile")
  else
    stream | /usr/bin/time -v -o "$tfile" "$@" >/dev/null 2>&1 || true
    # "Maximum resident set size (kbytes): 12345"
    rss_kb=$(awk -F': ' '/Maximum resident set size/ {print $2}' "$tfile")
  fi
  end=$(now)
  rm -f "$tfile"
  secs=$(awk -v s="$start" -v e="$end" 'BEGIN{printf "%.1f", e - s}')
  awk -v l="$label" -v s="$secs" -v b="$total" -v r="${rss_kb:-0}" \
    'BEGIN{printf "%-12s %8.1f s  %7.1f MB/s  peak RSS %6.1f MB\n", l, s, (s > 0 ? b / 1000000 / s : 0), r / 1024}'
}

case "$FMT" in
  san) measure pgn-extract "$C_BIN" --quiet -o/dev/null ;;
  *)   measure pgn-extract "$C_BIN" -W"$FMT" --quiet -o/dev/null ;;
esac
measure pgnx "$GO_BIN" convert --quiet -f "$FMT" -o /dev/null
