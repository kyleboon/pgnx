# pgnx Phase 4 Implementation Plan — duplicate detection (`pgnx dedupe`)

> **Status: COMPLETE** (verified 2026-11-05; commits `5716d96`..`HEAD` on `main`). Spec: [`docs/superpowers/specs/2026-11-04-pgnx-phase4-dedup-design.md`](../specs/2026-11-04-pgnx-phase4-dedup-design.md). Tracking: beads epic `pgnx-phase4` (one child task per task below).
> Every Phase 4 golden target byte-matches the oracle (test-duplicates, test-noduplicates, test-nounique, test-ZZ, test-checkfile, test-fuzzydepth 0/3/4/5, test-AA through a translated argument file). The nine-combination differential sweep over the golden inputs and TWIC issue 1500 is clean apart from the pre-existing `nested-comment.pgn` class and one designed divergence (`--fuzzy-depth N` on games without ply N, where the C tool reads an uninitialised hash); the per-file TWIC corpus run with `-D` is 745/749, the four being the documented "missing result" class amplified by `-D`. The `--same-setup` collection and a TWIC issue are identical to the C binary. Whole-corpus single-process `-D` run (3.6M games): peak RSS 273 MB (pgnx) vs 117 MB (C), wall 35 min vs 8 min. See CONFORMANCE.md "Duplicate detection (Phase 4)".

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking. Claim the matching beads task (`bd update <id> --claim`) before starting one and close it when its commit lands.

**Goal:** `pgnx dedupe` drops, diverts or keeps only the duplicate games of its inputs, seeded by check files, with exact or fuzzy (ply-depth) identity and optional same-setup suppression, composing with every `filter` criterion and output; every Phase 4 golden target byte-matches the C oracle.

**Architecture:** A ninth library package `dedup` (the seen-set, the game key, the setup set) that only `cmd/pgnx` imports; a one-method `SetupSeen` hook in `filter.Options` at the `check_duplicate_setup` point of `Evaluate`; `output.Writer.WriteComment`; a `dedupe` command built on the `run` struct with a third destination and a check-file pass before the inputs. The pipeline is unchanged.

**Tech Stack:** Go (pinned in `mise.toml`), cobra (CLI only), standard library. Reference C code: `~/code/pgn-extract` (flat layout: `hashing.c`, `grammar.c` `deal_with_game`, `apply.c` `play_moves`/`apply_move_list`, `argsfile.c`, `lex.c` for the check-file ordering). Line numbers below refer to that tree as of 2026-11-04.

## Global Constraints

Unchanged from Phases 1–3, plus the Phase 4 specifics:

- Module `github.com/kyleboon/pgnx`; GPL-3.0; cobra only under `cmd/`; library packages never `os.Exit`, never panic on user input, never write to stdout/stderr.
- Every task: `mise run ci` clean before committing (build, vet, gofmt, tidy, race tests, goreleaser check). Conventional commit messages.
- Oracle precedence when ambiguous: golden `outfiles/` → C source → never guess. Check `CONFORMANCE.md` before "fixing" any output difference.
- **Streaming (ADR 0003):** the only new cross-game state is `dedup.Log` (one map entry per *registered* game, about 40 bytes) and `dedup.SetupSet`; no slice of games anywhere.
- **Dependency direction:** `dedup` imports `chess`, `pgn`, `replay`; only `cmd/pgnx` imports it; `filter` reaches it through its own `SetupSeen` interface; `pipeline` never imports it.
- **Side-effect discipline:** `dedup` never mutates a game. The `From`/`First found in` lines are written by the destination, not attached to the game.
- **Pipeline purity:** `pipeline.Process` gains no state. The visit happens in the command's route, after `Process` and before `Selector.Decide`.

## Oracle target → pgnx invocation map

Inputs and oracles are under `cmd/pgnx/testdata/golden/{infiles,outfiles}` already; Task 7 adds the translated argument file.

| C target | C command (on `infiles/`) | pgnx args | oracle |
|---|---|---|---|
| test-duplicates | `-C -dtest-d-dupes.pgn -otest-d-unique.pgn --quiet fischer.pgn petrosian.pgn` | `dedupe --no-comments --duplicates D -o U fischer.pgn petrosian.pgn` | test-d-dupes.pgn + test-d-unique.pgn |
| test-noduplicates | `-D -otest-DD-unique.pgn --quiet fischer.pgn petrosian.pgn` (and `--noduplicates`) | `dedupe -o U fischer.pgn petrosian.pgn` | test-DD-unique.pgn |
| test-nounique | `-U -otest-UU-unique.pgn --quiet fischer.pgn petrosian.pgn` (and `--nounique`) | `dedupe --only-duplicates -o U fischer.pgn petrosian.pgn` | test-UU-unique.pgn |
| test-ZZ | `-C -Z -dtest-ZZ-dupes.pgn -otest-ZZ-unique.pgn --quiet fischer.pgn petrosian.pgn` | as test-duplicates (`-Z` not ported; oracles are byte-identical to test-d's) | test-ZZ-dupes.pgn + test-ZZ-unique.pgn |
| test-checkfile | `-cpetrosian.pgn -D -TpPetrosian -otest-c-out.pgn --quiet fischer.pgn`; then `-cclist.txt …` | `dedupe --check-file petrosian.pgn --player Petrosian -o O fischer.pgn` (once; the list-file run is the same command) | test-c-out.pgn |
| test-fuzzydepth | 4× `-C --fuzzydepth {0,3,4,5} -dtest-fuzzydepthN-dupes.pgn -otest-fuzzydepthN-unique.pgn --quiet test-fuzzydepth.pgn` | 4× `dedupe --no-comments --fuzzy-depth N --duplicates D -o U test-fuzzydepth.pgn` | test-fuzzydepthN-{dupes,unique}.pgn |
| test-AA | `-C -Aargslist.txt --quiet` (argsfile: `--novars`, `-s`, `-d test-AA-dupes.pgn`, `--output test-AA-unique.pgn`, `-t` White "Fischer" / Black "Petrosian", `-f gamefiles.txt`) | `dedupe --no-comments --quiet @argslist.pgnx.txt` in a temp cwd, with the translated file | test-AA-dupes.pgn + test-AA-unique.pgn |
| `--deletesamesetup` | *no C test target* | `dedupe --same-setup` over a hand-written three-game FEN collection | CLI test + differential run |

---

### Task 1: prefactor — criteria flags shared by `filter` and `dedupe`

**Files:**
- Modify: `cmd/pgnx/filter.go`, `cmd/pgnx/run.go`
- Test: `cmd/pgnx/filter_test.go` (existing tests must pass unchanged)

**Interfaces:**
- `func addCriteriaFlags(cmd *cobra.Command)` — everything `newFilterCmd` registers beyond `addOutputFlags`/`addECOFlags` (criteria, Selector, `--unmatched`, `--suppress-matched`).
- `func criteriaOptions(cmd *cobra.Command) (*filter.Options, filter.SelectorOptions, error)` — today's `filterOptions`, renamed.
- `func (r *run) routeBySelector(sel *filter.Selector, suppressMatched bool, disposition func(n int, o pipeline.Outcome) (matched bool)) route` — or equivalent: the Selector-driven route `runFilter` builds inline, with a hook that lets `dedupe` override the matched bit before `Decide`. `filter` passes `func(_, o) bool { return o.Matched }`.

- [x] **Step 1: Extract** without changing behaviour; `runFilter` becomes `criteriaOptions` + `newRun` + `routeBySelector`.
- [x] **Step 2: Verify** `go test ./cmd/pgnx/...` green; `go run ./cmd/pgnx filter --help` unchanged (diff the help text before and after).
- [x] **Step 3: Commit** — `refactor(cli): share filter's criteria flags and Selector route for the dedupe command`

---

### Task 2: `dedup` package — key, seen-set, setup set

**Files:**
- Create: `dedup/doc.go`, `dedup/key.go`, `dedup/log.go`, `dedup/setup.go`
- Test: `dedup/key_test.go`, `dedup/log_test.go`, `dedup/setup_test.go`, `dedup/testdata/fuzzydepth.pgn` (copy of the golden input)

**Interfaces (from the spec's Library API):**
- `type Key struct{ Final, Cumulative uint64 }`; `func KeyOf(r *replay.Result) Key` — `Final` is the last `Position.Placement`, `Cumulative` the wrapping sum of `Positions[1:]`'s `Placement` (`apply.c:1036`, `1332`; a game with no moves has `Final` = the start placement and `Cumulative` 0).
- `func FuzzyKey(r *replay.Result, depth int) (uint64, bool)` — depth 0 → `(Final, true)`; else the placement after the move whose absolute ply number is `depth` (`apply.c:968`: first move's ply = `2*(Fullmove-1) + (black?1:0) + 1`), `false` when the game has no such ply.
- `type Options struct{ Fuzzy bool; FuzzyDepth int }`; `func NewLog(o Options) *Log`; `func (l *Log) Visit(r *replay.Result, source string) (origin string, duplicate bool)`; `func (l *Log) Len() int`. Internals: `map[Key]uint32` (exact) and `map[uint64]uint32` (key-only, first registered wins) over a `[]string` of sources; storage rule per `hashing.c previous_occurance`.
- `type SetupSet struct{…}`; `func (s *SetupSet) Seen(fen string) bool` — `""` is the standard start (`standard_start_seen`); else `chess.FromFEN(fen).Zobrist()` (an unparsable FEN is treated as unseen and not registered: Normalise has already rejected it upstream).

- [x] **Step 1: Failing tests.** `KeyOf`/`FuzzyKey` on fools-mate and on a FEN game starting at move 21 (depth 3 → `ok=false`, depth 42 → the position after that ply); `Log` tables from `test-fuzzydepth.pgn` (depth 0: game 4 duplicates game 1; depth 3: game 3; depth 4: games 2 and 3; depth 5: game 4) and the exact case (identical games are duplicates; a transposition is not); origin reporting across two sources; `SetupSet` (same position, different clocks → seen; different side to move → not; standard start twice → seen).
- [x] **Step 2: Implement.**
- [x] **Step 3: Commit** — `feat(dedup): game keys, the seen-set and the setup set (hashing.c)`

---

### Task 3: `filter` — the same-setup hook; `output` — `WriteComment`

**Files:**
- Modify: `filter/filter.go` (`Options.SameSetup`, `Evaluate`), `output/output.go`, `output/json.go`, `pipeline/process.go` (doc comment on `Config.Criteria` only)
- Test: `filter/filter_test.go`, `output/output_test.go`

**Interfaces:**
- `type SetupSeen interface{ Seen(fen string) bool }` in `filter`; `Options.SameSetup SetupSeen` (nil: off).
- `Evaluate`: immediately after the SetUp-tag filter and before any replay-dependent check (`grammar.c:1216-1219`), `if o.SameSetup != nil && o.SameSetup.Seen(fenTag) { return false }`, with `fenTag` the `FEN` tag value or `""`.
- `func (w *Writer) WriteComment(text string) error` — writes `{ text }` and a newline in the movetext formats (`print_str`/`terminate_line`), nothing under `Options.JSON`; `text` is written verbatim.

- [x] **Step 1: Failing tests.** A recording `SetupSeen` stub: a game that fails `--player` is never offered; one that fails `--min-moves` is offered; a game offered and reported seen is unmatched and no later criterion runs (stub a sequence criterion that would match). `WriteComment` in `san` and under JSON.
- [x] **Step 2: Implement.** Update the `Config.Criteria` doc: a non-nil `SameSetup` makes `Process` order-dependent.
- [x] **Step 3: Commit** — `feat(filter,output): same-setup criterion hook; Writer.WriteComment`

---

### Task 4: `pgnx dedupe` — drop, divert, only-duplicates

**Files:**
- Create: `cmd/pgnx/dedupe.go`
- Modify: `cmd/pgnx/run.go` (`toDuplicates`, `duplicates destination`, the comment lines), `cmd/pgnx/root.go` (register the command), `cmd/pgnx/destinations.go` if the comment lines need a destination method
- Test: `cmd/pgnx/dedupe_test.go`, `cmd/pgnx/conformance_test.go`

**Interfaces:**
- `newDedupeCmd()`: `addOutputFlags`, `addECOFlags`, `addCriteriaFlags`, plus `--duplicates FILE`, `--only-duplicates`, `--check-file FILE` (StringArray, wired in Task 5), `--fuzzy-depth N` (Task 6), `--same-setup` (Task 6).
- `runDedupe`: `criteriaOptions` → `newRun` → open `--unmatched` and `--duplicates` → `dedup.NewLog` → route: `if o.Matched { origin, dup := log.Visit(o.Replay, item.Source) … }` then the disposition per the spec's step 5, `sel.Decide(n, disposition)`, and `toDuplicates` for a matched duplicate when `--duplicates` is set. The route needs the item's source name: extend `route`'s signature (or the `run.game` call) to carry it.
- `run.write` for `toDuplicates`: before the game, when comments are kept and not JSON, `{ From: <source> }` on a change of source since the last duplicate written, then `{ First found in: <origin> }` (`grammar.c:1283-1300`; `stdin` → `_stdin_`).
- Usage errors: `--duplicates` equal to `--output` or `--unmatched`.

- [x] **Step 1: Failing golden rows.** test-duplicates (two files), test-noduplicates, test-nounique, test-ZZ (same command as test-duplicates, comment says why). CLI tests: `--only-duplicates --duplicates D` sends duplicates to D and nothing to the output; the `From`/`First found in` lines with comments kept over two inputs; `--unmatched` receives dropped duplicates; the summary line counts as the C tool's would (`N games matched out of M`).
- [x] **Step 2: Implement.**
- [x] **Step 3: Commit** — `feat: pgnx dedupe drops, diverts or keeps only duplicate games (-D, -d, -U)`

---

### Task 5: `--check-file`

**Files:**
- Modify: `cmd/pgnx/dedupe.go`, `cmd/pgnx/run.go` (a `seed(files []string)` pass before `exec`), `cmd/pgnx/games.go` if the iterator needs a check-file mode
- Test: `cmd/pgnx/dedupe_test.go`, `cmd/pgnx/conformance_test.go`

**Interfaces:**
- `--check-file FILE` (repeatable, PGN files only; MIGRATION documents `@FILE` for lists).
- Seed pass: every check file in order, before the first input, each game Observe → Process → `log.Visit` when matched, routed `nowhere`; never counted by the Selector or the report's summary; a check file that cannot be opened is a usage error (exit 1, `must_open_file`); a game in a check file that fails to parse or replay is logged but does not fail the run.

- [x] **Step 1: Failing tests.** Golden test-checkfile. CLI: a check-file game that fails the criteria does not seed (a later identical input game is a first occurrence); check files are read first even when named after the inputs on the command line; `--first-game`/`--stop-after` count only input games; a missing check file is exit 1.
- [x] **Step 2: Implement.**
- [x] **Step 3: Commit** — `feat(dedupe): --check-file seeds the seen-set before the inputs (-c)`

---

### Task 6: `--fuzzy-depth` and `--same-setup`

**Files:**
- Modify: `cmd/pgnx/dedupe.go`
- Test: `cmd/pgnx/dedupe_test.go`, `cmd/pgnx/conformance_test.go`, `cmd/pgnx/testdata/same-setup.pgn` (three FEN games: two of one position with different clocks, one of another, plus a standard-start game twice)

- [x] **Step 1: Failing tests.** Golden test-fuzzydepth (four two-file runs, each its own `runCLI`). CLI: `--same-setup` over the collection keeps one game per start position and the first standard game; a same-setup game is unmatched (goes to `--unmatched`), is not in `--duplicates`, and is never offered to the seen-set; `--fuzzy-depth -1` is exit 1.
- [x] **Step 2: Implement** (`Options.SameSetup = &dedup.SetupSet{}` on the criteria; `dedup.Options{Fuzzy: set, FuzzyDepth: n}`).
- [x] **Step 3: Differential** for `--same-setup` against the C binary (`PGNX_CMD=dedupe C_FLAGS='--deletesamesetup' PGNX_FLAGS='--same-setup'` over the collection and a TWIC issue); record the result in CONFORMANCE.
- [x] **Step 4: Commit** — `feat(dedupe): --fuzzy-depth positional identity and --same-setup suppression`

---

### Task 7: test-AA, differential sweep, docs, close-out

**Files:**
- Add: `cmd/pgnx/testdata/golden/infiles/argslist.pgnx.txt` (translation of `argslist.txt`; comment at the top names the original)
- Modify: `cmd/pgnx/conformance_test.go` (test-AA in a temp cwd; drop the "Phase 4" exclusion comment), `MIGRATION.md`, `CONFORMANCE.md`, `README.md`, `CLAUDE.md`, `GLOSSARY.md`, `docs/adr/0002-…md` (amendment), this plan's banner, the spec's status line.

- [x] **Step 1: test-AA.** Translate the argsfile per MIGRATION's rules (`--no-variations`, `--duplicates test-AA-dupes.pgn`, `--output test-AA-unique.pgn`, `--tag White "Fischer"`, `--tag Black "Petrosian"`, the two inputs); run `dedupe --no-comments --quiet @argslist.pgnx.txt` in a temp cwd; compare both files. Close the note on `pgnx-phase4`.
- [x] **Step 2: Differential.** `scripts/difftest.sh` with `PGNX_CMD=dedupe` over the golden inputs and one TWIC issue for `-D`, `-d`, `-U`, `-c`, `--fuzzydepth 0`, `--fuzzydepth 10`, `-D -TpX`, `-D -n`; `scripts/twicdiff.sh` with `-D` over the whole corpus (note peak RSS of both tools in this step's checkbox). Fix with oracle justification or document. *Done 2026-11-05: sweep 502 identical / 11 divergent (8 `nested-comment.pgn`, 3 the documented fuzzy-depth class); `twicdiff` (now taking `PGNX_CMD`/`C_FLAGS`/`PGNX_FLAGS`) 745/749; one-process whole corpus with `-D`: pgnx peak RSS 273 MB in 2122 s, C 117 MB in 498 s. Documented in CONFORMANCE.md.*
- [x] **Step 3: `MIGRATION.md`.** Rows for `-D`/`--noduplicates`, `-d`/`--duplicates`, `-U`/`--nounique`, `-c`/`--checkfile` (lists via `@FILE`), `--fuzzydepth`, `--deletesamesetup`, `-Z` (not ported, in-memory); "Not yet ported" shrinks accordingly; the argument-file section gains the `-c` list translation.
- [x] **Step 4: `CONFORMANCE.md`.** Section "Duplicate detection (Phase 4)": identity by placement key rather than weak hash (equality-only, no emitted values); `-Z` not ported; `-c` list-by-suffix not ported; `--json` duplicates file is a valid array without the `From`/`First found in` lines; the uninitialised fuzzy key of a FEN game whose first ply is beyond the depth; the `First found in` source under fuzzy matching when a short game's final key equals a later game's fuzzy key (first registered wins); whatever Step 2 surfaces.
- [x] **Step 5: Docs.** `README.md` dedupe examples; `CLAUDE.md` (`dedup` as the ninth package in the layer list, `dedupe` in the subcommand list and the `run` callers, phase status line); `GLOSSARY.md` (duplicate, first occurrence, seen-set, check file, same setup — with the words to avoid); ADR 0002 amendment: the stateful `SetupSeen` criterion hook and the rule that the duplicate visit lives in the route.
- [x] **Step 6: Full check** — `mise run ci`; flip this plan's banner to **COMPLETE** with the verification date and commit range; set the spec's status to Implemented.
- [x] **Step 7: Commit** — `docs: dedupe migration and conformance notes; mark Phase 4 complete`

---

## Self-Review (before marking complete)

- **Spec coverage:** identity (exact, fuzzy) → 2; disposition (`-D`/`-d`/`-U`) → 4; check files → 5; same-setup → 3/6; comments in the duplicates file → 4; argument-file target → 7; docs/ADR → 7.
- **Ordering parity:** visit after all criteria and before the Selector (pinned by the `--first-game` and `--unmatched` CLI tests); same-setup between the SetUp-tag filter and the replay-stage checks (pinned by the recording stub); check files before inputs regardless of order (pinned); `From` line only on a change of source (pinned by the two-input test).
- **Side-effect discipline:** grep `dedup/` for writes to `g.Tags`/`g.Moves`: none. The comment lines are destination output, not game content.
- **Streaming:** no slice of games; `Log` holds one entry per registered game; `twicdiff` RSS recorded in Task 7 Step 2.
- **Pipeline purity:** `pipeline/` diff is a doc comment only.
