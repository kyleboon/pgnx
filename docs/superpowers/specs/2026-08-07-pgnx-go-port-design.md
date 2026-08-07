# pgnx: Go Port of pgn-extract — Phase 1 Design

**Date:** 2026-08-07
**Status:** Approved
**Module:** `github.com/kyleboon/pgnx`

## Context

pgn-extract is David J. Barnes' C tool (1994–present, ~21.5k lines) for searching,
filtering, and reformatting chess games in PGN. It is semantically deep — it replays
every game on an internal board with full legality checking — and battle-hardened
against decades of malformed real-world PGN. Its main structural liabilities are a
monolithic `GlobalState` struct (~100 fields), a hand-rolled parser with no formal
grammar, and exit-on-error memory management, all of which make it hard to test,
extend, or embed.

The goal is a modern replacement: a reusable Go library plus an idiomatic CLI,
reaching near-full feature parity over several phases. The C source at
`~/code/pgn-extract` remains untouched as the reference implementation and test
oracle.

## Decisions (from brainstorming)

| Question | Decision |
|---|---|
| Motivation | Modernize the tool; parity matters |
| CLI style | Idiomatic redesign with subcommands (cobra); old flags mapped in `MIGRATION.md`, not supported |
| Chess core | Written from scratch, porting C semantics (no third-party chess library) |
| Repo | New sibling repo `~/code/pgnx`, module `github.com/kyleboon/pgnx` |
| Porting approach | Idiomatic re-architecture with the C code as behavioral oracle, enforced by golden-corpus and differential testing |
| Phase 1 scope | Core parse + replay/validation + output formats; filtering, position matching, ECO, and dedup deferred to later phases |

## Architecture

```
github.com/kyleboon/pgnx
├── chess/          # Board, pieces, move generation, legality, Zobrist hash.
│                   # Zero PGN knowledge. Chess960-aware. Usable standalone.
├── pgn/            # Streaming lexer + parser: io.Reader → *pgn.Game.
│                   # Lenient (C-oracle semantics). No board knowledge.
├── replay/         # Applies a parsed game to a board: SAN disambiguation,
│                   # legality validation, per-move resolved squares, check
│                   # status, FEN/Zobrist per position.
├── output/         # Formatters: san, source, lalg, halg, elalg, xlalg, uci,
│                   # epd, fen. Line width, tag roster, element stripping.
└── cmd/pgnx/       # CLI: cobra subcommands, thin composition of the above.
```

This makes the C code's implicit layering (lex/grammar → decode/map/apply →
output) explicit. The parse/replay split is the key boundary: `pgn` yields the
game as written (even if illegal); `replay` determines what it means on a board.
Later phases (filtering, dedup) consume replayed games and hashes without
touching the parser. The `GlobalState` monolith is replaced by per-package
options structs composed by the CLI.

### C-to-Go source mapping

| C source | Go package |
|---|---|
| lex.c, grammar.c, tokens.h, taglist.h | `pgn` |
| decode.c, map.c (movegen/legality), defs.h (Board) | `chess` |
| apply.c (replay parts), decode.c (SAN semantics) | `replay` |
| output.c | `output` |
| main.c, argsfile.c | `cmd/pgnx` |
| zobrist.c | `chess` (Polyglot-compatible Zobrist) |
| lists.c, end.c, fenmatcher.c, moves.c, eco.c, hashing.c | later phases |

## Library API

Streaming iterator over `io.Reader`; million-game files in constant memory:

```go
scanner := pgn.NewScanner(r, pgn.ParserOptions{...})
for scanner.Next() {
    game := scanner.Game()      // *pgn.Game
    _ = game.ParseErrors()      // recoverable oddities, collected not fatal
}
err := scanner.Err()            // fatal I/O / unrecoverable state
```

Core `pgn` types preserve source fidelity (re-emittable as parsed):
- `Game`: tag map + ordered tag list, `[]*Move`, prefix comment, source line range
- `Move`: SAN text as written, NAGs, comments, `[]*Variation`
- `Variation`: prefix/suffix comments + `[]*Move`, recursive

Replay produces the semantic layer:

```go
result, err := replay.Game(game, replay.Options{Chess960: true})
// result.Positions[i]: board state after move i (FEN, Zobrist)
// result.Moves[i]:     resolved from/to, piece, capture, check/mate
// err:                 replay.IllegalMoveError{Ply, SAN} on first illegal move
```

Output:

```go
w := output.NewWriter(os.Stdout, output.Options{
    Format: output.SAN, LineWidth: 75, TagRoster: output.SevenTag,
})
w.WriteGame(game, result) // result required for formats needing board state
```

`chess` is independently usable: `NewBoard()`, `FromFEN(s)`, `LegalMoves()`,
`Apply(move)`, `Zobrist()`.

Error philosophy: library code never exits; typed/sentinel errors throughout;
the C tool's lenient parsing is preserved by collecting recoverable issues per
game rather than failing the stream. Multilingual piece letters and the C
lexer's permissive move-text acceptance are ported as behavior, verified by the
oracle tests.

## CLI

Cobra-based subcommands; every verb reads files-or-stdin, writes stdout-or-`--output`:

```
pgnx convert [flags] [file...]   # parse, validate, re-emit (the workhorse)
    -f, --format san|source|lalg|halg|elalg|xlalg|uci|epd|fen
    -o, --output FILE   --append
    -w, --line-width N
    --seven-tag-roster | --roster TAG,...
    --no-comments --no-nags --no-variations --no-move-numbers --no-results
    --keep-broken       # emit games that fail validation (default: skip + log)

pgnx validate [file...]          # syntax + move legality; report errors
    --summary                    # counts only

pgnx info [file...]              # stats: game count, tag coverage, ply counts
```

Root-level: `--quiet`, `--log FILE`, `--version`, generated help.
Exit codes: 0 success, 1 usage error, 2 input problems found.
Later phases add verbs (`filter`, `dedupe`, `eco`, `split`), not flags on
existing ones.

`MIGRATION.md` maps old flags to new invocations
(`-r` → `pgnx validate`, `-Wuci` → `pgnx convert -f uci`, `-7` →
`--seven-tag-roster`, ...).

Dependencies: `spf13/cobra` for the CLI; standard library otherwise.

## Testing

1. **Unit tests** per package, table-driven: lexer token streams, parser
   structure, `chess` movegen against known perft counts, FEN round-trips,
   Zobrist against published Polyglot key values, formatter output.
2. **Golden corpus conformance**: vendor the C repo's `test/infiles` and
   `test/outfiles` into `testdata/`. A Go harness maps each relevant C test
   target (from the C repo's `test/Makefile`) to a pgnx invocation and diffs
   against oracle files. Phase 1 activates the parse/output targets
   (`test-7`, `test-W*`, `test-C`, `test-N`, `test-V`, width/roster tests);
   later phases activate theirs.
3. **Differential testing**: a script (optionally CI) runs the C binary and
   pgnx over large real-world PGN samples and diffs normalized output.
   Divergences become new golden cases or entries in `CONFORMANCE.md`
   (documented intentional differences: exit codes, error wording, flag
   surface).

## Phasing

- **Phase 1 (this spec):** repo scaffold, `chess`/`pgn`/`replay`/`output`
  packages, `pgnx convert|validate|info`, golden + differential harness.
  Done when Phase-1 golden targets pass and a differential run over a large
  corpus is clean (modulo `CONFORMANCE.md`).
- **Phase 2:** `pgnx filter` — tag criteria, bounds, checkmate/material
  filters (ports `lists.c`, `end.c`).
- **Phase 3:** position/variation matching + ECO (`fenmatcher.c`, `moves.c`,
  `eco.c`).
- **Phase 4:** `pgnx dedupe` + Polyglot hash tooling (`hashing.c`).

Each phase gets its own spec → plan → implementation cycle.

## Non-goals (Phase 1)

- Drop-in C flag compatibility (documented mapping instead)
- Tag/position/material filtering, ECO classification, duplicate detection
- JSON output, ChessMaster format, argsfile support (revisit by demand in
  later phases)
- Byte-identical error/log messages with the C tool (game output must match;
  diagnostics may differ, per `CONFORMANCE.md`)
