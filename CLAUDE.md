# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

pgnx is a Go port of pgn-extract (David J. Barnes' C tool for processing chess PGN files) — a from-scratch re-architecture, not a transliteration. It ships a reusable library (`chess`, `pgn`, `replay`, `output`) and a cobra-based CLI (`cmd/pgnx`) with subcommands `convert`, `validate`, and `info`. GPL-3.0, same as the original. Currently Phase 1 (parsing, replay/validation, output formatting); filtering, position matching, ECO classification, and dedup are later phases — see `docs/superpowers/specs/` for the design spec and phasing plan.

## Commands

```sh
go build ./...                      # build everything
go build -o pgnx ./cmd/pgnx        # build the CLI binary
go test ./...                      # full test suite (fast, a few seconds)
go test ./pgn -run TestName        # single test
go test ./chess -run TestPerft -v  # move-generation perft suite
go vet ./...

# Differential test against the C implementation (requires the C binary,
# default $HOME/code/pgn-extract/build/bin/pgn-extract; override with C_BIN):
scripts/difftest.sh <input.pgn> [format]
```

## Architecture

The library is four layers with a strict dependency direction; the CLI sits on top:

1. **`chess`** — self-contained chess engine: board representation, move generation, legality, check/mate/draw status, FEN, Zobrist hashing. Has *no* PGN knowledge and is independently usable.
2. **`pgn`** — lexer → parser → `Scanner`, producing `*pgn.Game`. Preserves source fidelity: SAN exactly as written, comments, NAGs, variations, tag order. It does **not** validate move legality. `pgn.Scanner` streams one game at a time (`bufio.Scanner`-style) so million-game files run in constant memory — keep that property when touching the parse path.
3. **`replay`** — applies a parsed game's moves to a `chess.Board`, resolving SAN against legal moves and producing a `replay.Result` with per-move detail (FEN, checks, captures, from/to squares) that non-Source output formats need. A game that fails replay is "broken" and only emittable in Source format.
4. **`output`** — renders a `*pgn.Game` + `replay.Result` into the supported formats (`san`, `halg`, `lalg`, `elalg`, `xlalg`, `uci`, `epd`, Source) with line-wrapping rules that match the C tool.

CLI exit codes: 0 = success, 1 = usage/flag error, 2 = one or more games failed to parse/validate/convert (intentionally unlike the C tool, which mostly exits 0).

## Conformance to the C oracle

The C pgn-extract implementation is the behavioral reference, and this shapes how changes are made and verified:

- **Byte-for-byte parity** on emitted PGN/format output wherever both tools implement a feature. Diagnostics wording, log formatting, and exit codes are explicitly *not* held to parity.
- `cmd/pgnx/conformance_test.go` is a golden-corpus harness over `cmd/pgnx/testdata/golden/` (vendored from the C repo's `test/infiles`/`test/outfiles`). Each table row mirrors a target in the C repo's `test/Makefile`, with the original C command kept as a comment so drift is auditable — preserve that convention when adding rows.
- `CONFORMANCE.md` documents all known intentional divergences and open concerns. **Before "fixing" any output difference from the C tool, check CONFORMANCE.md first** — several divergences are deliberate (e.g. NAG/comment interleaving order, null-move rejection, `--keep-broken` always emitting Source format). New divergences found in testing should either be fixed with oracle justification or documented there, not silently patched around.
- `MIGRATION.md` maps the C tool's flags to pgnx subcommands/flags; update it when adding CLI surface.
