# pgnx: Go Port of pgn-extract — Phase 4 Design (duplicate detection: `pgnx dedupe`)

**Date:** 2026-11-04
**Status:** Implemented (2026-11-05; see the plan for the commit range; plan: [`docs/superpowers/plans/2026-11-04-pgnx-phase4-dedup.md`](../plans/2026-11-04-pgnx-phase4-dedup.md); tracking: beads epic `pgnx-phase4`)
**Module:** `github.com/kyleboon/pgnx`
**Builds on:** [Phase 1](2026-08-07-pgnx-go-port-design.md), [Phase 2](2026-10-28-pgnx-phase2-filter-design.md), [Phase 3](2026-10-29-pgnx-phase3-positions-eco-design.md) (all complete), ADR 0002 (pipeline), ADR 0003 (streaming), ADR 0004 (argument files)

## Context

Phases 1–3 judge every game on its own. pgn-extract's remaining feature
family judges a game against the games that came before it in the run:

- **Duplicate detection** (`hashing.c` `previous_occurance`): a game is a
  duplicate when its final position *and* the sum of the positions it
  passed through equal those of a game already seen. `-D` drops
  duplicates, `-d FILE` diverts them to a file, `-U` drops the first
  occurrences instead and keeps only the duplicates.
- **Check files** (`-c FILE`): games that seed the seen-set before the
  inputs are read, so that "duplicate" means "already in the check files"
  as well as "already in this run". They are never written or counted.
- **Fuzzy matching** (`--fuzzydepth N`): two games are duplicates when they
  reach the same position after ply `N` (or, with `N` = 0, at the end),
  whatever moves got them there.
- **Same-setup suppression** (`--deletesamesetup`): drop a game whose
  starting position (its `FEN` tag, or the standard start) has been seen
  before, before the moves are even replayed. A collection-of-studies
  tool: with ordinary games it keeps only the first game.
- **`-Z`**: the same seen-set kept in a temporary file instead of memory.

The Phase 1 design named Phase 4 as "`pgnx dedupe` + Polyglot hash
tooling (`hashing.c`)". The hash tooling has since shipped piecemeal:
`chess.Board.Zobrist` and `WeakHash`, `replay.Position.Placement`,
`filter --hash`, `convert --add-hash-code`/`--hash-comments`. What is new
in Phase 4 is the cross-game state and the routing around it. Every golden
target the C `test/Makefile` has for the family (`test-duplicates`,
`test-noduplicates`, `test-nounique`, `test-ZZ`, `test-checkfile`,
`test-fuzzydepth`, `test-AA`) is already vendored under
`cmd/pgnx/testdata/golden/` and excluded from the conformance harness as
"Phase 4".

## Decisions

| Question | Decision |
|---|---|
| Verb or flags | A new subcommand, `pgnx dedupe`, per the Phase 1 rule that later phases add verbs. It takes **every `filter` flag** (criteria, Selector, `--unmatched`, output, transform, ECO) plus the dedup flags, because in the C tool `-D`/`-d`/`-U`/`-c` compose with every criterion (`test-checkfile` is `-c … -D -TpPetrosian`) and the duplicate check sits *after* all criteria in `deal_with_game`. `dedupe` with no dedup flags is `filter` plus "drop duplicates". The criteria flag registration and option parsing move out of `newFilterCmd` into functions both commands call (a prefactor, no behaviour change). |
| Game identity | `dedup.Key{Final, Cumulative}`: the placement key of the final main-line position and the wrapping `uint64` sum of the placement keys after each main-line move (`apply.c:1036`, `1332`). pgnx uses `replay.Position.Placement` (Phase 3's `PlacementKey`, an XOR over the Polyglot piece table) rather than `chess.Board.WeakHash`: both are placement-only 64-bit keys whose only observable property here is equality, and `Placement` is already computed by every replay. `WeakHash` stays what it is, the value behind the emitted `HashCode` tag. Variations never contribute (C: `mainline` only). |
| Fuzzy identity | `--fuzzy-depth N`: the placement key after the move with **absolute ply number `N`** (`apply.c:968`, `1039`: `plies` starts at the first move's ply number, so a game from a FEN at move 21 has no ply 3); `N` = 0 means the final position. The table and the two-pass lookup are ported as `hashing.c` has them: an exact pass on `(Final, Cumulative)`, then a fuzzy pass on the key alone; a game long enough to have ply `N` is stored as `(fuzzyKey, 0)`, a shorter one as its exact pair (`hashing.c previous_occurance`). Where C reads an uninitialised `fuzzy_duplicate_hash` (a FEN game whose first ply is beyond `N`), pgnx treats the game as having no fuzzy key and uses the exact pair: defined where the oracle is undefined, documented. |
| Where the check runs | In the command, not the pipeline. `pipeline.Process` stays stateless and concurrency-safe (ADR 0002: "everything order-sensitive stays with the caller"); `dedupe`'s route visits the seen-set with the outcome's replay **after** `Process` and **before** the Selector's decision, which is the C order (`previous_occurance` runs once all criteria pass, before `first_game` and the `--select-only` bookkeeping). Only games that are not Rejected and that matched the criteria are visited, as in C; a Broken game kept with `--keep-broken` is visited with its partial replay (`apply.c:1280`). |
| Same-setup is a criterion | `--same-setup` is the one stateful check that sits *inside* the C criteria chain (`grammar.c:1217`: after `check_setup_tag`, before `apply_move_list`), so a game that fails a tag criterion never registers its start position while one that fails a length bound does. To keep that order exactly, `filter.Options` gains `SameSetup SetupSeen`, a one-method interface (`Seen(fen string) bool`, `""` for the standard start) that `Evaluate` calls at that point; `dedup.SetupSet` implements it with the full Polyglot hash of the FEN (`hashing.c check_duplicate_setup` uses `generate_zobrist_hash_from_fen`, not the weak hash) and a `standardStartSeen` flag. A nil `SameSetup` leaves `Evaluate` pure; a non-nil one makes `Process` order-dependent, which is documented on `Config.Criteria` and is a constraint on the `--jobs` work (`pgnx-perf.2`). `--same-setup` is a `dedupe` flag only. |
| Routing | Three outcomes per visited game: *first occurrence*, *duplicate*, and the destination each goes to. Default (`-D`): duplicates are suppressed, which in C means they are **unmatched** (`grammar.c:1238`: `game_matches` stays false, so `-n` receives them and the summary does not count them). `--duplicates FILE` (`-d`): duplicates are matched and written to FILE instead of the output; the output file keeps the first occurrences. `--only-duplicates` (`-U`): first occurrences are suppressed (unmatched) and duplicates are matched and written to the output, or to `--duplicates FILE` when both are given, which the C tool also allows. Suppressed games go through `Selector.Decide(n, false)` so `--unmatched`, `--stop-after`, `--first-game` and the summary line all see what the C tool's do. |
| The duplicates file's comments | When comments are kept, the C tool writes `{ From: <input> }` once per input file that contributes a duplicate and `{ First found in: <file> }` before every duplicate (`grammar.c:1283-1300`), both into the `-d` file only; a game first seen on stdin is reported as `_stdin_`. pgnx writes the same lines, through a new `output.Writer.WriteComment(text)` (a `{ … }` line in the movetext formats, nothing under `--json`, where C would write them raw into the array and break it: documented divergence). `--no-comments` suppresses them, as `-C` does; every golden target uses `-C`. |
| Check files | `--check-file FILE` (repeatable) names a PGN file whose games seed the seen-set: they are read **before** every input, whatever the command-line order (`lex.c:1488`), run through the same pipeline with the same criteria (C runs `deal_with_game` on them, so only games that satisfy the criteria register), and are never written, counted or reported in the summary; their names are what `First found in` reports. The C tool's rule that a `-c` argument without a `.pgn` suffix is a list of file names is **not** ported: lists are argument files (`@FILE` with one `--check-file` line each, ADR 0004). MIGRATION.md says so. |
| `-Z` (virtual hash table) | Not ported. The seen-set is an in-memory map keyed by the 16-byte `Key` with a `uint32` source index (roughly 40 bytes per registered game in a Go map: 150 MB for the 3.6M-game TWIC corpus), which satisfies ADR 0003's "a few words per game". `test-ZZ`'s oracle files are byte-identical to `test-duplicates`'s (verified 2026-11-04), so the harness runs `test-ZZ` as a second row of the same command. A spill-to-disk table is the escape hatch ADR 0003 names; it is Out, owned by `pgnx-perf`, and becomes a `--jobs`-era concern. |
| `--eco-split` and `--split-variants` | The duplicates file is one file; first occurrences go through the main destination, ECO-split or not, exactly as `select_output_file` routes only matched games. A game is visited once, on the source game's replay, before it is split; its parts go to the same destination. |
| Diagnostics | None per game, as in C (the only trace of a duplicate is the comment in the `-d` file). The summary line is `filter`'s. Exit codes unchanged. |

## Scope

### In

- **`pgnx dedupe`** with `filter`'s complete flag set plus:
  `--duplicates FILE` (`-d`), `--only-duplicates` (`-U`),
  `--check-file FILE` (`-c`, repeatable), `--fuzzy-depth N`
  (`--fuzzydepth`), `--same-setup` (`--deletesamesetup`). The default
  disposition (no `--duplicates`, no `--only-duplicates`) is `-D`.
- **`dedup` package**: `Key`, `KeyOf`, `FuzzyKey`, `Log` (the seen-set with
  exact and fuzzy lookup and the source-name table), `SetupSet`.
- **`filter`**: the `SameSetup` hook in `Options`/`Evaluate` at the
  `check_duplicate_setup` point; `TagNames` unchanged.
- **`output`**: `Writer.WriteComment`.
- **CLI prefactor**: `addCriteriaFlags`, `criteriaOptions` (today's
  `filterOptions`) shared by `filter` and `dedupe`; `run` gains a third
  destination (`toDuplicates`) and the check-file pass.
- **Golden rows**: test-duplicates, test-noduplicates, test-nounique,
  test-ZZ, test-checkfile (two runs, one oracle), test-fuzzydepth (depths
  0, 3, 4, 5), test-AA (argument file translated into pgnx's grammar,
  vendored beside the C original). The "Phase 4" exclusion comment in
  `conformance_test.go` goes away.
- **Docs**: MIGRATION (`-D`/`-d`/`-U`/`-c`/`-Z`/`--fuzzydepth`/
  `--deletesamesetup` rows; "Not yet ported" shrinks to the known rest),
  CONFORMANCE (the divergences above), README, CLAUDE.md (ninth package,
  sixth subcommand), GLOSSARY (duplicate, first occurrence, seen-set, check
  file, same setup), ADR 0002 amendment (the stateful criterion hook).

### Out (with owner)

| Item | Owner |
|---|---|
| Spill-to-disk seen-set (`-Z`) | `pgnx-perf` |
| `--jobs` with dedupe: visiting in emission order; `--same-setup` under parallel evaluation | `pgnx-perf.2` |
| `-c` list-file magic by file suffix | not ported; MIGRATION points at `@FILE` |
| A `--duplicates` file under `--json` carrying the `From`/`First found in` lines | not ported (C writes invalid JSON) |
| Retiring MIGRATION/CONFORMANCE once everything is ported | `pgnx-docs-retire-cdocs` |

## Architecture

```
github.com/kyleboon/pgnx
├── dedup/     NEW — hashing.c
│     key.go       Key, KeyOf(r), FuzzyKey(r, depth)        (apply.c:968-1077, 1332)
│     log.go       Log: Visit(r, source) (origin, dup), Len   (previous_occurance, LogTable)
│     setup.go     SetupSet: Seen(fen) bool                   (check_duplicate_setup)
├── filter/    + Options.SameSetup SetupSeen; Evaluate calls it after the SetUp-tag criteria
├── output/    + Writer.WriteComment(text)                    (grammar.c print_str/terminate_line)
├── pipeline/  unchanged (Config.Criteria doc notes the hook)
└── cmd/pgnx/  + dedupe.go; criteria flags shared with filter; run: toDuplicates, check-file pass
```

Dependency direction: `dedup` imports `chess`, `pgn`, `replay`; nothing
imports `dedup` except `cmd/pgnx`. `filter` sees `dedup.SetupSet` only
through its own `SetupSeen` interface. `pipeline` does not import `dedup`.

## Library API

```go
// Identity.
k := dedup.KeyOf(r)                       // r *replay.Result; Final = last Placement, Cumulative = Σ Placement after each move
fk, ok := dedup.FuzzyKey(r, 4)            // placement after absolute ply 4; ok=false when r has no such ply; depth 0 → Final

// The seen-set.
log := dedup.NewLog(dedup.Options{FuzzyDepth: 0, Fuzzy: false})
origin, dup := log.Visit(r, "games/a.pgn")   // registers r unless an equivalent game was seen; dup=true returns that game's source
n := log.Len()

// Same-setup.
set := &dedup.SetupSet{}
seen := set.Seen(fenTag)                  // "" = the standard start; registers on first sight
var _ filter.SetupSeen = set
```

`Visit` is the whole of `previous_occurance`: exact pass, fuzzy pass when
enabled, insert on miss with the fuzzy-or-exact storage rule, `_stdin_`
substitution left to the caller (the CLI passes the source name the
iterator gives it, and maps `"stdin"` to `_stdin_` when it builds the
comment). `Log` and `SetupSet` are not safe for concurrent use; that is
the point of them.

## CLI

```
pgnx dedupe [filter flags] [dedup flags] [file...]

Disposition (default: duplicates are dropped, i.e. -D):
    --duplicates FILE        -d FILE   write duplicates to FILE ("-" for stdout); first occurrences to --output
    --only-duplicates        -U        write only duplicates; first occurrences are dropped
                                       (with --duplicates: duplicates to FILE, nothing to --output)
Seed:
    --check-file FILE        -c FILE   games already "seen" (repeatable; read before the inputs; never written)
Identity:
    --fuzzy-depth N          --fuzzydepth N   duplicates share the position after ply N (0: the final position)
    --same-setup             --deletesamesetup   drop a game whose starting position was seen before
```

Dropped games are unmatched: `--unmatched FILE` receives them, the
summary line does not count them. Usage errors (exit 1): `--fuzzy-depth`
below 0; `--duplicates` naming the `--output` or `--unmatched` file; a
`--check-file` that cannot be opened (C: `must_open_file`, exit 1). `-D`
with `-d` is a C usage error that pgnx cannot express, since dropping is
the default and `--duplicates` replaces it.

## Evaluation order (what the port must preserve)

Per game, in `deal_with_game` order (`grammar.c:1214-1300`), with the
Phase 4 steps marked:

1. Normalisation (`consistent_FEN_tags`), tag criteria, rated-winner,
   SetUp-tag filter — unchanged.
2. **`--same-setup`** (`check_duplicate_setup`): with a `FEN` tag, the
   full Polyglot hash of the FEN is looked up and registered; without one,
   the standard start is registered once. Seen → the game is unmatched
   from here (nothing later runs, as the `&&` chain stops).
3. Replay, the replay-time criteria, length bounds, sequences, material,
   piece count, checkmate, repetition, ECO criteria, commented — unchanged.
   The pipeline computes nothing new: `KeyOf`/`FuzzyKey` read the replay's
   positions afterwards.
4. **Visit** (`previous_occurance`), only when steps 1–3 matched: exact
   lookup of `(Final, Cumulative)`; if fuzzy, a second lookup of the fuzzy
   key (depth 0: `Final`); on a miss, store `(fuzzyKey, 0)` when fuzzy and
   the game has ply `N`, else the exact pair, with the game's source index.
   A check-file game stops here: registered, never routed.
5. **Disposition**: duplicate and dropping → unmatched; first occurrence
   and `--only-duplicates` → unmatched; otherwise matched.
6. Selector: `--first-game`, `--select-only`/`--skip-matching`,
   `--stop-after`, `--game-limit` — unchanged, on the disposition.
7. Write: a matched duplicate with `--duplicates` goes to that file,
   preceded (comments kept, not JSON) by `{ From: <input> }` when the input
   file differs from the last one that contributed a duplicate, and by
   `{ First found in: <origin> }`; every other matched game goes to the
   output (ECO-split or not); unmatched games to `--unmatched`.

Check-file pass, before step 1 of the first input: every `--check-file`
in command-line order, each game through steps 1–4 with the run's
criteria and a route that registers and returns `nowhere`; parse failures
in a check file are reported but do not fail the run (C logs and carries
on); the game numbers the Selector sees start at the first input.

## Testing

1. **Unit tests** in `dedup`: `KeyOf` on fools-mate (Final equals the last
   `Placement`, Cumulative the sum); `FuzzyKey` on a standard game and on
   a FEN game starting at move 21 (depth 3 → `ok=false`; depth 42 → the
   position after that ply); `Log` tables lifted from `test-fuzzydepth.pgn`
   (depth 0: game 4 duplicates game 1; depth 3: game 3; depth 4: games 2
   and 3; depth 5: game 4) and the exact case (same moves twice; a
   transposition is *not* an exact duplicate); origin reporting (first
   registered source wins; two sources); `SetupSet` with two FEN games of
   the same position with different clocks (equal), different side to
   move (different), and the standard start seen twice. In `filter`: a
   stub `SetupSeen` that records calls proves a game failing a tag
   criterion is never offered and one failing a length bound is. In
   `output`: `WriteComment` in `san`, `--no-comments` (caller's job, so
   the test is on the CLI), `--json` (nothing written).
2. **Golden conformance** rows (C command kept as a comment):
   test-duplicates (`-C -d D -o U` → `dedupe --no-comments --duplicates D
   -o U`, two files), test-noduplicates (`-D -o U`), test-nounique (`-U -o
   U`), test-ZZ (same as test-duplicates, `-Z` noted as not ported),
   test-checkfile (`-c petrosian.pgn -D -TpPetrosian` → `dedupe
   --check-file petrosian.pgn --player Petrosian`; the `clist.txt` run is
   the same pgnx command, its oracle shared), test-fuzzydepth (four
   two-file runs at depths 0, 3, 4, 5), test-AA (`-C -A argslist.txt
   --quiet` → `dedupe --no-comments @argslist.pgnx.txt --quiet` with a
   translated argument file: `--no-variations`, `--duplicates`,
   `--output`, two `--tag` lines, the two input files). `--same-setup` has
   no oracle file: a CLI test over a three-game FEN collection, pinned by
   a differential run against the C binary.
3. **Differential**: `scripts/difftest.sh` with `PGNX_CMD=dedupe` over the
   golden inputs and a TWIC issue for `-D`, `-d`, `-U`, `-c`,
   `--fuzzydepth 0/10`, `--deletesamesetup`, `-D -TpX`, `-D -n`; the whole
   TWIC corpus with `-D` through `scripts/twicdiff.sh` (the one run where
   the seen-set is large: record its RSS in the plan's close-out).

## Non-goals (Phase 4)

- Diagnostics parity: the C tool is silent about duplicates and so is
  pgnx; no "N duplicates" summary is added.
- Parallel processing: `dedupe` is sequential by nature; `--jobs` is
  `pgnx-perf.2`'s problem and this spec records the two constraints it
  must meet (visit in emission order; `--same-setup` mid-chain).
- Cross-run persistence of the seen-set: `--check-file` is the C tool's
  answer and pgnx's.
