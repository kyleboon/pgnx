# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

pgnx is a Go port of pgn-extract (David J. Barnes' C tool for processing chess PGN files) — a from-scratch re-architecture, not a transliteration. It ships a reusable library (`chess`, `pgn`, `replay`, `filter`, `eco`, `transform`, `dedup`, `pipeline`, `output`) and a cobra-based CLI (`cmd/pgnx`) with subcommands `convert`, `filter`, `dedupe`, `split`, `validate`, and `info`. GPL-3.0, same as the original. Phases 1 (parsing, replay/validation, output formatting), 2 (game selection: `pgnx filter`), 2b (output transforms and options: the `--ply-count`/`--drop-ply`/`--fen-comments`/`--no-tags`/`--split-variants` family on `convert` and `filter`, and `pgnx split`), 3 (positional and move-sequence matching, ECO classification via `--eco-file`/`--eco-split` on `convert` and `filter`) and 4 (duplicate detection: `pgnx dedupe`) are complete — see `docs/superpowers/specs/` for the design specs and phasing plan.

## Commands

```sh
mise install                        # pinned Go + GoReleaser (mise.toml)
mise run ci                         # exactly what CI runs: build, vet, gofmt -l, go mod tidy -diff, test -race, goreleaser check
mise run snapshot                   # full release dry-run into dist/, no tag or token needed
go build ./...                      # build everything
go build -o pgnx ./cmd/pgnx        # build the CLI binary
go test ./...                      # full test suite (fast, a few seconds)
go test ./pgn -run TestName        # single test
go test ./chess -run TestPerft -v  # move-generation perft suite
mise run bench                     # move-generation and replay benchmarks with allocs/op and allocs/ply (not in ci)
go vet ./...

# Differential test against the C implementation (requires the C binary,
# default $HOME/code/pgn-extract/build/bin/pgn-extract; override with C_BIN):
scripts/difftest.sh <input.pgn> [format]
# Filter runs: PGNX_CMD selects the subcommand, C_FLAGS/PGNX_FLAGS add
# verbatim arguments to each binary:
PGNX_CMD=filter C_FLAGS='-TpFischer' PGNX_FLAGS='--player Fischer' scripts/difftest.sh <input.pgn>
# Corpus differential over a directory of PGN files, one process per file in
# parallel (JOBS, default 8); keeps a .diff per divergent file. The local
# TWIC corpus (twic/, gitignored, ~3.6M games) takes about 3 minutes:
scripts/twicdiff.sh [pgn-dir] [out-dir]   # defaults twic/twic/pgn, twic/diff
# twicdiff takes PGNX_CMD/C_FLAGS/PGNX_FLAGS like difftest, e.g.
# PGNX_CMD=dedupe C_FLAGS='-D' scripts/twicdiff.sh twic/twic/pgn twic/diff-dedupe
# Streaming check (ADR 0003): stream N GB of TWIC (default 2) on stdin through
# pgnx and the C binary, report wall time, MB/s and peak RSS for both; slow:
scripts/bigstream.sh [size-gb] [pgn-dir]   # e.g. scripts/bigstream.sh 0.05 for a smoke test
```

## Architecture

The library is nine packages with a strict dependency direction; the CLI sits on top:

1. **`chess`** — self-contained chess engine: board representation, move generation, legality, check/mate/draw status, FEN, Zobrist hashing. Has *no* PGN knowledge and is independently usable.
2. **`pgn`** — lexer → parser → `Scanner`, producing `*pgn.Game`. Preserves source fidelity: SAN exactly as written, comments, NAGs, variations, tag order. It does **not** validate move legality. `pgn.Scanner` streams one game at a time (`bufio.Scanner`-style) so million-game files run in constant memory — keep that property when touching the parse path.
3. **`replay`** — applies a parsed game's moves to a `chess.Board`, resolving SAN against legal moves and producing a `replay.Result` with per-move detail (checks, captures, from/to squares, per-position hashes and clocks) that non-Source output formats need. No FEN text is built per ply: `Result.BoardAt(i)` gives one position's board on demand and a consumer that needs every ply's FEN walks the typed moves from `StartBoard`, as the C tool builds FEN components only for the EPD/FEN formats and FEN comments (`apply.c`). The null move `--` (`chess.NullMove`) is accepted in variations always and in the main line only with `Options.AllowNullMoves` (`--allow-null-moves`). A game that fails replay is "broken" and only emittable in Source format.
4. **`filter`** — game selection: tag criteria (`-t` line grammar, `TagMatcher`), length bounds, odds games (`--odds`), end-state/property predicates, material-balance patterns, positional criteria (`PositionCriteria`: `-x` move lines, FENs, Polyglot hashes, FEN patterns with labels and material constraints, walked per ply as `apply.c` `play_moves` does), move sequences (`Sequence`: `-v`/`-P`/`--vanywhere`, textual on the moves as written), result-tag consistency, and the `Selector` behind `--select-only`/`--stop-after` etc. `Evaluate` runs the checks in the C tool's `deal_with_game` order, trusts the tags `pipeline.Normalise` has already corrected, and is the only other place that mutates a parsed game (Result/MaterialMatch/MatchLabel tags, `--mark-matches` comments). Imports `chess`, `pgn`, `replay`; only `cmd/pgnx` imports it.
5. **`eco`** — ECO classification (`eco.c`): `Load` builds a table of opening lines from an ECO PGN file, `Table.Classify` matches a replayed game's positions against it, `Apply` replaces the ECO/Opening/Variation/SubVariation tags. A tag-adding transform, not a criterion, so it sits beside `filter` (same imports; does not import `filter`); the CLI classifies before `filter.Evaluate` so ECO criteria see the new tags, and `--eco-split` routes output by `FilePrefix`.
6. **`transform`** — the output-side rewrites of a game (output.c `format_game`, apply.c `rewrite_game`): `transform.Apply(g, r, Options)` prepends the `--line-numbers` comment, drops plies (`--drop-ply`, `--drop-before`; the remainder gets FEN/SetUp tags and a fresh replay), adds the player tags from a `PlayerInfo` file (`--add-elo-tags`, `--add-fide-id-tags`) and the PlyCount/TotalPlyCount/HashCode tags (`chess.Board.WeakHash` is the C tool's hash behind HashCode); `transform.SplitVariations(g, depth)` is grammar.c `split_variants`, returning the main line and one variation-free game per variation (`--split-variants`). Mutates the game, never selects it; sits beside `eco` and `filter` (imports `chess`, `pgn`, `replay`). Options that change only the rendering (`--ply-limit`, `--fen-comments`, `--hash-comments`, `--evaluation`, `--final-fen`, `--comment-lines`, `--no-tags`, `--no-checks`) are `output.Options`, not transforms.
7. **`dedup`** — duplicate detection (`hashing.c`): `dedup.Key`/`KeyOf` is a game's identity (the final main-line placement key and the wrapping sum of the per-move placement keys, `apply.c` `play_moves`; variations never contribute), `FuzzyKey` the placement after an absolute ply number, `Log.Visit` the seen-set (`previous_occurance`: exact pass, fuzzy pass under `Options.Fuzzy`, the fuzzy-or-exact storage rule, the origin source), `SetupSet` the set of starting positions behind `--same-setup` (`check_duplicate_setup`, full Polyglot hash of the FEN), which `filter.Evaluate` consults through its `SetupSeen` hook at the `check_duplicate_setup` point. Never mutates a game; only `cmd/pgnx` imports it (`filter` sees it only through `SetupSeen`; `pipeline` never does). Imports `chess`, `replay`. The CLI visits `Outcome.SourceReplay`, the game as parsed, so `--drop-ply` does not change a game's identity (C hashes inside `apply_move_list`, before `rewrite_game`).
8. **`pipeline`** — pgn-extract's per-game semantics between parser and writer, in `deal_with_game` order (ADR 0002): `pipeline.New(Config{ECO, KeepBroken, AllowNullMoves, Criteria, Transform, SplitVariations, SplitDepth}).Process(g)` runs `Normalise` (grammar.c `check_result`/`consistent_FEN_tags`: Result `1/2`→`1/2-1/2`, Result filled from the movetext token, SetUp `1` added to a FEN game, SetUp without FEN or a bad FEN rejects the game), replay (with the missing-result move-dropping rule as a `Warning`), the broken-ness decision against keep-broken, ECO classification, `filter.Evaluate` (always, nil criteria matching all) and `transform.Apply` (last, so criteria see the game as parsed; a game too short for `--drop-ply` is Rejected; under `SplitVariations` the game is split first and each split game replayed and transformed on its own), returning an `Outcome{Game, Replay, Status OK|Broken|Rejected, Reason, Warnings, Matched}` whose `Emissions()` lists every game to write (one, or the split games). `Process` touches only its game and read-only config, so it is safe to call concurrently; everything order-sensitive (tag-registry `Observe`, `Selector`, routing, writing) stays with the caller. `convert`, `filter`, `dedupe`, `split` and `validate` are callers, through the `run` struct in `cmd/pgnx/run.go` (one per invocation: tag registry, pipeline, log writer, destinations; its `game` method does Observe -> Process -> route -> write, and the commands differ only by their `route` function and, for `split`, the destination). `dedupe`'s route visits the `dedup.Log` after `Process` and before the Selector, in `deal_with_game`'s order, and a `--check-file` pass runs the same per-game sequence over the check files first, routed nowhere. The `pipeline` gains no cross-game state: `Config.Criteria` with a `SameSetup` is the one order-dependent case, documented there; imports `chess`, `pgn`, `replay`, `filter`, `eco`, `transform`, never `output`.
9. **`output`** — renders a `*pgn.Game` + `replay.Result` into the supported formats (`san`, `halg`, `lalg`, `elalg`, `xlalg`, `uci`, `epd`, Source) with line-wrapping rules that match the C tool, including the generated per-move comments and the ply limit; `Options.JSON` renders the same formats as pgn-extract's `--json` objects instead (one array per Writer, ended by `Writer.Close`, which the destinations call). `output.TagRegistry` orders unrecognised tags in run-wide first-seen order, as the C tool does; the CLI feeds it every parsed game.

CLI exit codes: 0 = success, 1 = usage/flag error, 2 = one or more games failed to parse/validate/convert (intentionally unlike the C tool, which mostly exits 0).

## Conformance to the C oracle

The C pgn-extract implementation is the behavioral reference, and this shapes how changes are made and verified:

- **Byte-for-byte parity** on emitted PGN/format output wherever both tools implement a feature. Diagnostics wording, log formatting, and exit codes are explicitly *not* held to parity.
- `cmd/pgnx/conformance_test.go` is a golden-corpus harness over `cmd/pgnx/testdata/golden/` (vendored from the C repo's `test/infiles`/`test/outfiles`). Each table row mirrors a target in the C repo's `test/Makefile`, with the original C command kept as a comment so drift is auditable — preserve that convention when adding rows.
- `CONFORMANCE.md` documents all known intentional divergences and open concerns. **Before "fixing" any output difference from the C tool, check CONFORMANCE.md first** — several divergences are deliberate (e.g. NAG/comment interleaving order, null-move rejection, `--keep-broken` always emitting Source format). New divergences found in testing should either be fixed with oracle justification or documented there, not silently patched around.
- `MIGRATION.md` maps the C tool's flags to pgnx subcommands/flags; update it when adding CLI surface.


## CI and releasing

- `.github/workflows/ci.yml` runs `mise run ci` on ubuntu, macos and windows for pushes to `main` and PRs, then a snapshot release on ubuntu. Make CI green by making `mise run ci` pass locally; the two are the same command.
- **Releasing is a human act**: a maintainer pushes a `v*` semver tag and `.github/workflows/release.yml` runs GoReleaser (`.goreleaser.yaml`), publishing the GitHub Release and the Homebrew cask in `kyleboon/homebrew-tap`. Never create or push a `v*` tag yourself. Tags not on `main` are refused. See `docs/adr/0001-release-via-tag-push-and-homebrew-tap.md`.
- The version comes from `-ldflags -X main.version=...` (set by GoReleaser) with a `debug.ReadBuildInfo` fallback in `cmd/pgnx/version.go`; there is no version constant to bump.
- Before touching `.goreleaser.yaml` run `mise run release-check`, then `mise run snapshot` and inspect `dist/`.
- Actions are pinned to commit SHAs with a `# vX.Y.Z` comment; Dependabot bumps them. Keep the pin and the comment together when editing.
- The C-oracle differential (`scripts/golden-difftest.sh`, expected divergences in `scripts/golden-difftest-known.txt`) runs only via the manual `C oracle difftest` workflow.

## Agent skills

### Issue tracker

Issues live in Beads (`bd`), not GitHub Issues. See `docs/agents/issue-tracker.md`.

### Triage labels

Default vocabulary: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` at the root and ADRs under `docs/adr/`. See `docs/agents/domain.md`.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:1105d646 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/core-concepts/sync-concepts.md for details and anti-patterns.

## Agent Context Profiles

The managed Beads block is task-tracking guidance, not permission to override repository, user, or orchestrator instructions.

- **Conservative (default)**: Use `bd` for task tracking. Do not run git commits, git pushes, or Dolt remote sync unless explicitly asked. At handoff, report changed files, validation, and suggested next commands.
- **Minimal**: Keep tool instruction files as pointers to `bd prime`; use the same conservative git policy unless active instructions say otherwise.
- **Team-maintainer**: Only when the repository explicitly opts in, agents may close beads, run quality gates, commit, and push as part of session close. A current "do not commit" or "do not push" instruction still wins.

## Session Completion

This protocol applies when ending a Beads implementation workflow. It is subordinate to explicit user, repository, and orchestrator instructions.

1. **File issues for remaining work** - Create beads for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **Handle git/sync by active profile**:
   ```bash
   # Conservative/minimal/default: report status and proposed commands; wait for approval.
   git status

   # Team-maintainer opt-in only, unless current instructions forbid it:
   git pull --rebase
   git push
   git status
   ```
5. **Hand off** - Summarize changes, validation, issue status, and any blocked sync/commit/push step

**Critical rules:**
- Explicit user or orchestrator instructions override this Beads block.
- Do not commit or push without clear authority from the active profile or the current user request.
- If a required sync or push is blocked, stop and report the exact command and error.
<!-- END BEADS INTEGRATION -->
