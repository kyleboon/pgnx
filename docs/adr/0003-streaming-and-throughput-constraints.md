# 0003: Memory is bounded per game and throughput must match the C tool

Status: accepted (2026-11-02); amended 2026-11-12 (pgnx-perf.1.6), 2026-11-13 (pgnx-perf.3,
pgnx-perf.4, pgnx-perf.9), 2026-11-19 (pgnx-perf.6) and 2026-11-23 (pgnx-perf.5), baseline table and
follow-up tickets below

pgnx must process PGN inputs of 30 GB and more. Two constraints follow that
the code cannot show on its own, so they are recorded here and checked by
`scripts/bigstream.sh`, which runs pgnx and pgn-extract over a multi-gigabyte
stream and reports peak RSS and throughput for both.

1. **Memory is bounded by the largest single game, never by the input.** The
   parser yields one game at a time, the pipeline judges it, the writer emits
   it, and nothing holds on to it afterwards. A feature that needs run-wide
   state (duplicate detection is the planned one) must keep that state
   proportional to a few words per game, with a spill-to-disk option as the
   escape hatch, and must never collect games. Streaming inside a game is
   out of scope: no downstream stage can judge a partial game. Measured on
   2026-11-02: live heap after GC stayed at 0.2 MB across 272,046 games.

2. **Single-threaded throughput matches pgn-extract on the same input.**
   Parallel processing (`--jobs`) is a bonus on top of that, not a way to
   reach it. The baseline is `scripts/bigstream.sh` over the same 232 MB
   TWIC slice (the first 100 issues, twic1000 to twic1099, 232,960,652
   bytes, which the script reports as "232 MB" and earlier notes called
   222 MiB; `scripts/bigstream.sh 0.232960652`), `convert` to `/dev/null`,
   on an Apple M1, median of three runs; MB/s divides decimal megabytes
   by wall time, RSS is the peak resident set in MiB:

   | date | pgn-extract | pgnx | ratio | state |
   |---|---|---|---|---|
   | 2026-11-02 | 19 s, 12 MB/s, 3 MB RSS | 159 s, 1.4 MB/s, 15 MB RSS | 8.4x | replay generated every legal move three times per ply, allocating each time (19 GB allocated for a 20 MB input) |
   | 2026-11-12 | 15.7 s, 14.8 MB/s, 2.8 MB RSS | 24.8 s, 9.4 MB/s, 13.4 MB RSS | 1.58x | after pgnx-perf.1: one SAN-directed generation per ply into a reused buffer, no per-ply FEN (832 ns/ply, 1.1 allocs/ply in `BenchmarkGame`) |
   | 2026-11-13 | 15.7 s, 14.8 MB/s, 2.9 MB RSS | 19.0 s, 12.3 MB/s, 13.5 MB RSS | 1.21x | after pgnx-perf.3: the board carries its placement key, updated per piece placed or removed, so no per-ply board scan for the hashes (568 ns/ply, 1.1 allocs/ply in `BenchmarkGame`, from 832) |
   | 2026-11-13 | 15.7 s, 14.8 MB/s, 2.9 MB RSS | 15.9 s, 14.7 MB/s, 13.7 MB RSS | 1.01x | after pgnx-perf.4: the mate test after a check stops at the checked side's first legal reply and castling text probes its one move, instead of each generating every legal move with a legality probe per candidate (433 ns/ply, 1.1 allocs/ply in `BenchmarkGame`, from 568) |
   | 2026-11-13 | 15.7 s, 14.8 MB/s, 2.9 MB RSS | 11.3 s, 20.6 MB/s, 14.7 MB RSS | 0.72x | after pgnx-perf.9 (ADR 0005): the board is bitboards, attack tests are table lookups and magic-bitboard slider lookups, and candidate generation is a mask intersection, so no probe walks the board (232 ns/ply, 1.1 allocs/ply in `BenchmarkGame`, from 433) |
   | 2026-11-19 | 15.7 s, 14.8 MB/s, 2.9 MB RSS | 9.7 s, 24.0 MB/s, 14.0 MB RSS | 0.62x | after pgnx-perf.6: the lexer scans a 64 KB block buffer of bytes instead of runes through `bufio`, a token's text is one sub-slice converted once with tag names, moves, move numbers and NAGs interned, and the parser allocates `Move`s from a per-game chunked slab; a scanner-only pass over the slice (`BenchmarkScannerCorpus` in `pgn`, pointed at the slice by `PGNX_SCAN_CORPUS`, an absolute path) went from 3.40 s and 268 allocs/game to 1.84 s and 21 allocs/game (72.8 M to 5.8 M allocations), `BenchmarkScanner` from 263 to 34 allocs/game |
   | 2026-11-23 | 15.7 s, 14.8 MB/s, 2.9 MB RSS | 7.8 s, 29.9 MB/s, 14.1 MB RSS | 0.50x | after pgnx-perf.5: a replay allocates only what its `Result` holds: the replayer and its move buffer come from a `sync.Pool` instead of being made per game, a line's SAN strings are sliced from one `strings.Builder` text instead of one string per ply, the replays of a move's variations moved off `MoveInfo` (88 to 64 bytes) into `Result.Variations(i)`, the standard start board is copied rather than parsed from FEN per game, and the SAN suffix trimming and piece-letter maps became byte switches; a convert of the slice allocates 5.33 GB in 24.0 M objects from 7.35 GB in 46.9 M (replay's share 2.52 GB from 4.35 GB), and `BenchmarkGame` went from 100 to 7.4 allocs/game, 1.1 to 0.08 allocs/ply, 18.9 to 11.5 KB/game and 229 to 154 ns/ply |

   A separate stage-timing run on 2026-11-12 over the same slice (best of
   three, total 24.6 s) spent 76% of its wall time in replay (18.7 s;
   parse 3.3 s, output 2.2 s). A sampled profile of a convert run put
   two full-board hash scans per ply at 26% and check detection by ray
   scan at 21% of all sampled user CPU; pgnx-perf.3 (done 2026-11-13)
   removed the hash scans, and pgnx-perf.4 (done 2026-11-13) the two
   full generations behind most check-detection calls, which took the
   `IsAttacked` share of a sampled convert run from 28.3% (measured
   after perf.3) to 17.3%. What remained of that share was the mailbox
   scan itself (84% of calls fell through all 18 knight, king and pawn
   probes into the eight ray walks); pgnx-perf.9 (done 2026-11-13, ADR
   0005) replaced the representation with bitboards, which took the
   attack-test share of a sampled convert run over the same slice from
   21.6% to 3.2% (both by the one method: `IsAttacked` plus its helpers
   among the collapsed leaf samples of a 10 s `sample` of the run, kernel
   waits excluded) and put pgnx ahead of the C tool for the first time.
   The top leaf samples were then the SAN suffix trimming in replay
   (`strings.TrimRight`, 8%), `Board.Apply` and `setPiece` (7.5%
   together), a map access and tiny allocations in the lexer, and the
   lexer's rune reads; pgnx-perf.6 (done 2026-11-19) removed the lexer's
   share (the scanner-only pass over the slice, `BenchmarkScannerCorpus`,
   is 1.84 s from 3.40 s, and
   lexing and parsing allocate 5.8 M objects over the slice from 72.8 M),
   and pgnx-perf.7 carries the output remainder. pgnx-perf.5 (done
   2026-11-23) took the replay allocations out of the per-game path: of
   the 7.35 GB a convert of the slice allocated, replay owned 4.35 GB
   (59%): 1.93 GB of `MoveInfo`s, 0.93 GB of `Position`s and 1.36 GB of
   move buffers made fresh for every game and dropped; afterwards the
   run allocates 5.33 GB in 24.0 M objects (from 46.9 M), replay 2.52 GB
   of it, which is the two per-game slices (a 64-byte `MoveInfo` and a
   40-byte `Position` per ply) and the SAN text. Interning tag values as
   well was measured and rejected: it halved the remaining allocations
   but hashing the values cost more time than it saved.

## Consequences

- Output destinations are buffered once per run and flushed at close.
- An `io.Pipe` test guards the "never reads ahead of the current game"
  property in the game iterator; RSS assertions stay out of `go test`.
- A performance epic, sequenced after the `replay.Position` rework, owns the
  move-generation redesign (done 2026-11-12), the hashing and check-detection
  follow-ups, and the parallel runner.
