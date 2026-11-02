# 0003: Memory is bounded per game and throughput must match the C tool

Status: accepted (2026-11-02)

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
   reach it. Baseline on 2026-11-02 over a 222 MB TWIC slice: pgn-extract
   12 MB/s at 3 MB RSS; pgnx 1.4 MB/s at 15 MB RSS. The gap is replay
   generating every legal move for every ply, three times per ply, with a
   fresh allocation each time (19 GB allocated for a 20 MB input), where the
   C tool generates only the candidates the SAN names, without allocating.

## Consequences

- Output destinations are buffered once per run and flushed at close.
- An `io.Pipe` test guards the "never reads ahead of the current game"
  property in the game iterator; RSS assertions stay out of `go test`.
- A performance epic, sequenced after the `replay.Position` rework, owns the
  move-generation redesign and the parallel runner.
