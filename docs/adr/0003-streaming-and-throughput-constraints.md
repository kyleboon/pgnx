# 0003: Memory is bounded per game and throughput must match the C tool

Status: accepted (2026-11-02); amended 2026-11-12 (pgnx-perf.1.6) and 2026-11-13 (pgnx-perf.3), baseline
table and follow-up tickets below

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

   A separate stage-timing run on 2026-11-12 over the same slice (best of
   three, total 24.6 s) spent 76% of its wall time in replay (18.7 s;
   parse 3.3 s, output 2.2 s). A sampled profile of a convert run put
   two full-board hash scans per ply at 26% and check detection by ray
   scan at 21% of all sampled user CPU; pgnx-perf.3 (done 2026-11-13)
   removed the hash scans, pgnx-perf.4 under the `pgnx-perf` epic carries
   the check-detection share, and pgnx-perf.5 to pgnx-perf.7 the
   allocation, lexer and output remainder.

## Consequences

- Output destinations are buffered once per run and flushed at close.
- An `io.Pipe` test guards the "never reads ahead of the current game"
  property in the game iterator; RSS assertions stay out of `go test`.
- A performance epic, sequenced after the `replay.Position` rework, owns the
  move-generation redesign (done 2026-11-12), the hashing and check-detection
  follow-ups, and the parallel runner.
