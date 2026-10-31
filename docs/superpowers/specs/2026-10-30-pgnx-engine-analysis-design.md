# pgnx: Engine Analysis Design (`pgnx analyze`, a port of uci-analyser)

**Date:** 2026-10-30
**Status:** Draft
**Module:** `github.com/kyleboon/pgnx`
**Builds on:** [Phase 1](2026-08-07-pgnx-go-port-design.md), [Phase 2](2026-10-28-pgnx-phase2-filter-design.md), [Phase 3](2026-10-29-pgnx-phase3-positions-eco-design.md) (all complete)
**Oracle:** David J. Barnes' *uci-analyser* (C++, 2013–2021, GPL-3.0, `~/code/uci-analyser`), **not** pgn-extract. Nothing in this document is covered by the pgn-extract golden corpus or by `CONFORMANCE.md`'s byte-parity rule; see [Conformance](#conformance).

## Context

uci-analyser is the companion tool pgn-extract's author wrote for the
"chessplag" research on detecting engine-assisted play. Its pipeline is:

```
games.pgn ──pgn-extract -Wuci──▶ uci moves ──analyse──▶ XML / annotated text
                                             │
                                             └── UCI engine (stockfish), MultiPV, fixed depth
```

It is about 1,900 lines in four units:

- `engine.cpp` — fork/exec a UCI engine with pipes; `uci` → scrape
  `id name` → `uciok` → `setoption UCI_AnalyseMode true`, `MultiPV N`, user
  `--setoption` pairs → `ucinewgame`/`isready`. Blocking, line-oriented.
- `analyse.cpp readGame` — a hand-rolled reader of tag lines + whitespace-
  separated UCI moves (the `-Wuci` shape). Two tags matter: `FEN` (start
  position; disables book skipping) and `BookDepth` (per-game override of
  `--bookdepth`, default 8 plies, the opening theory to skip).
- `analyse.cpp sendGame` — per ply past the book, for the selected colours
  (`--whiteonly`/`--blackonly`): `position startpos moves …`, `go depth N`;
  collect `info … multipv` lines whose `depth == N`, keep the latest per
  first move, order best-first with mate-aware comparison
  (`saveEvaluation`); if the played move is not among the MultiPV lines,
  re-run `go depth N searchmoves <played>` and merge. The guarantee that
  **the played move always has an evaluation** is the one real idea in the
  tool.
- Output: XML (`<gamelist><game><tags/><moves/><analysis …><move player>
  <played/><evaluation move value/>…`), where `value` is centipawns from
  the **side to move's** perspective, `mate N`, or a `L`/`U` bound suffix;
  `--annotate` / `--annotatePGN` print `move { eval } ( best { eval } )…`
  inline, in UCI move strings with no move numbers (PGN-shaped, not PGN).

Known weaknesses the port must not inherit: fixed depth only; one engine,
one game at a time; scores left in side-to-move perspective; an engine that
stops short of the requested depth (forced mate, one legal move) produces
*no* evaluation and an "Internal error"; no evaluation cache, so shared
opening positions are searched once per game; 100-byte static read buffer.

pgnx already owns both ends of that pipeline: `replay.Result` has every
position and resolved `chess.Move` (so the UCI move strings and FENs need
no `-Wuci` pre-pass), and `pgn.Move` holds comments, NAGs and variations
(so annotations can be written back as *real* PGN through `output`). This
spec adds the middle.

## Decisions

| Question | Decision |
|---|---|
| Is this a pgn-extract phase? | No. pgn-extract has no engine support, so this is a feature family beside the port, not Phase 4 (dedup keeps that name). Conformance obligations are to uci-analyser's XML only, and only where documented below. |
| Library shape | Two new packages. `uci` is the engine protocol (process, handshake, options, search) and imports `chess` only. `analysis` walks a replayed game, drives `uci`, and produces a `Report`; it also owns the PGN annotation transforms and the XML/JSONL writers. `analysis` imports `chess`, `pgn`, `replay`, `uci`; `filter` and `info` consume `analysis.Summary` but never start an engine themselves. Only `cmd/pgnx` composes. |
| Search unit | One `go` per analysed position (the position **before** each selected ply), with `MultiPV N`. The played move's line gives the score *after* the move; centipawn loss = best score − played score, both from the mover's perspective. Same as uci-analyser, one search per ply. A final extra search of the terminal position runs only when `--eval` is on, so the last move's `%eval` reflects the position it produced. |
| Played-move fallback | Kept: when the played move is absent from the MultiPV set, re-search with `searchmoves <played>` and merge, so every analysed ply has a `PlayedScore`. |
| Depth matching | **Diverges** from uci-analyser: per first move, keep the line with the highest depth seen before `bestmove` (ties: latest), not only `depth == N`. Fixes the early-stop failure. The XML writer records the achieved depth per move. |
| Limits | `Depth`, `Nodes`, `MoveTime` — exactly one. uci-analyser only had depth. |
| Score model | `uci.Score{Kind: CP|Mate, Value int, Bound}` with a total order (`Compare`) that ranks: winning mates (shorter first) > centipawns (higher first) > losing mates (longer first); this is uci-analyser's `saveEvaluation`/`worse_move` ordering made explicit. `Negate()` flips perspective. Stored as the engine reports it (side to move); `analysis` converts to white's perspective for `%eval` and to the mover's perspective for loss. For loss and ACPL any score is clamped to ±1000 cp (mate counts as 1000). |
| `%eval` comment | `{[%eval 0.35]}` / `{[%eval #3]}` / `{[%eval #-3]}`, white's perspective, pawns with two decimals, appended as the move's **last** comment. This is the de facto standard read by lichess, Scid and ChessBase. The parser side (`analysis.ParseEval`) also accepts the `,depth` suffix some tools write. |
| NAG marking | `--nags` assigns one assessment NAG from the mover's loss: `?!` ($6) at ≥ `--inaccuracy`, `?` ($2) at ≥ `--mistake`, `??` ($4) at ≥ `--blunder`; defaults 50 / 100 / 300 cp. Any existing $1–$6 on the move is replaced; other NAGs are kept. |
| Best lines | `--best-lines N` inserts up to N engine lines whose first move differs from the played move as RAVs after the move, converted to SAN through `chess`, each with a leading `{[%eval …]}` prefix comment. `--only-worse` restricts all annotation to plies whose loss is ≥ `--inaccuracy`. |
| Summary tags | `--summary-tags` adds `WhiteACPL`, `BlackACPL`, `WhiteBlunders`, `BlackBlunders`, `WhiteTop1`, `BlackTop1` (integer percent of analysed plies whose played move was the engine's first choice) and `Annotator` (`"<engine id>, depth 20"` / `", 2s/move"`). Tag order is `output.TagRegistry`'s first-seen rule, as for ECO. |
| Book depth | `--book-depth N` default **0** (analyse everything); a `BookDepth` tag overrides it per game exactly as uci-analyser did; both are ignored when the game has a `FEN` tag, also as uci-analyser did. The 8-ply default was a research choice, not a tool default users annotating games expect. |
| Broken games | A game that fails replay cannot be analysed and is skipped (counted toward exit 2). `--keep-broken` passes it through unannotated in Source format, matching `convert`. |
| Parallelism | `--parallel N` runs N engine processes; games are dispatched to the pool and emitted in **input order** through a bounded window of N in-flight games, so streaming memory stays O(N). Engine threads are the engine's business: `--set-option Threads=4`. |
| Cache | `--cache FILE`: append-only JSONL keyed by (`replay.Position.Zobrist`, engine id, limit string, MultiPV); loaded on start, appended as searches complete. The full Polyglot hash is the key (side to move, castling and en passant matter to an evaluation), not `Placement`. |
| Legacy XML | `-f xml` on `analyze` reproduces uci-analyser's element layout (`gamelist`, `game`, `tags`, `moves`, `analysis`, `move`, `played`, `evaluation`) so chessplag-style consumers keep working. Values stay in **side-to-move perspective** there, as in the original. Additions are attribute-only (`depth` on `evaluation`, `limit` on `analysis`). The `--annotate` text format is **not** ported: it is neither PGN nor XML and has no consumer. |
| JSONL | `-f jsonl`: one object per game (`tags`, `engine`, `limits`, `plies[]` with `ply`, `color`, `played`, `score`, `best`, `loss`, `lines[]`, `summary`). Scores in white's perspective, mates as `{"mate": n}`. |
| Engine failure | Cannot start / no `uciok` → usage/environment error, exit 1, before any game is read. Engine dies mid-run → the current and all remaining games are counted failed, exit 2; the pool does not restart engines (a restart loop hides the real problem). |
| Tests without an engine | The test suite never shells out to a real engine. `uci/internal/fakeuci` is a `chess`-powered stub (legal moves from `chess`, score = material in centipawns, mate from `chess` status, deterministic) that the test binary runs as itself via the `os.Args[0]` + env re-exec pattern. It speaks enough UCI for the handshake, options, `position`, `go` with every limit and `searchmoves`, `stop`, `quit`, and can be told to stop early or die on demand. |

## Scope

### In

- **`uci` package**: `Start`, handshake with id/option capture, `SetOption`,
  `NewGame`, `Search` (depth/nodes/movetime, MultiPV, searchmoves, bound
  flags, highest-depth-per-move collection), `Close`, context cancellation
  (`stop` then `quit`, kill on timeout), `Pool`.
- **`analysis` package**: `Analyzer.Game` → `Report` (per-ply lines, played
  score, best line, loss, achieved depth, colour); `Report.Summary` (ACPL,
  blunder counts, top-1 rate per colour); `Annotate` (eval comments, NAGs,
  best-line RAVs, summary tags, only-worse gating); `Summary` recovery from
  an already-annotated game (`FromGame`: summary tags if present, else
  `%eval` comments — top-1 rate is unavailable from comments alone);
  `WriteXML`, `WriteJSONL`; optional `Cache`.
- **`pgnx analyze`**: new subcommand (alias `analyse`) with all of
  `convert`'s output flags plus the engine, limit, scope, annotation and
  format flags below; `BookDepth` tag honoured; `--skip-analyzed`.
- **`pgnx filter`**: eval criteria read from existing annotation
  (`--max-acpl`, `--min-blunders`, `--min-top1`, each with an optional
  `white:`/`black:`/`either:` prefix, default `either:`), and `--engine` to
  compute the summary on the fly for games that lack it, after the existing
  criteria chain has passed.
- **`pgnx info`**: per-game ACPL / blunder / top-1 columns when annotation
  is present.
- **`pgnx convert --no-eval`**: strip `[%eval …]` comments (and comments
  left empty by that) while keeping everything else.
- `MIGRATION.md` gains a uci-analyser section; `CONFORMANCE.md` gains an
  "Engine analysis (uci-analyser)" section listing the divergences above.

### Out (with owner)

- uci-analyser's `--annotate` / `--annotatePGN` text formats (no consumer;
  `-f xml`, `-f jsonl` and real PGN cover every use).
- Engine-vs-engine play, pondering, `go infinite`, Syzygy/book probing →
  not a PGN tool's job.
- Win-percentage based accuracy (lichess 2020+) → possible later addition
  on top of `Report`; ACPL first.
- Annotating moves *inside* source variations → later; the main line only
  is what uci-analyser did and what `BookDepth`/colour selection is defined
  for.
- Re-analysing already-annotated games selectively (`--skip-analyzed` is
  all-or-nothing per game).
- pgn-extract's own `--evaluation` (static material comment) stays with the
  transforms epic; it is unrelated to engines.

## Architecture

```
github.com/kyleboon/pgnx
├── uci/                NEW  engine protocol; imports chess only
│     engine.go         Start, handshake, SetOption, NewGame, Search, Close
│     score.go          Score, Bound, Compare, Negate, Pawns
│     info.go           parse "info …" lines (depth, seldepth, multipv, score, nodes, nps, time, pv)
│     pool.go           Pool: N engines, Acquire/Release, ordered dispatch helper
│     internal/fakeuci  chess-powered stub engine for tests
├── analysis/           NEW  imports chess, pgn, replay, uci
│     analyze.go        Analyzer, Options, Game → Report; book depth, colour selection, searchmoves fallback
│     report.go         Report, Ply, Line, Summary, Report.Summary()
│     annotate.go       Annotate: %eval comments, NAGs, best-line RAVs, summary tags
│     eval.go           FormatEval / ParseEval, FromGame (summary from tags or %eval comments)
│     xml.go            WriteXML (uci-analyser layout)
│     jsonl.go          WriteJSONL
│     cache.go          Cache: JSONL load/append keyed by Zobrist+engine+limit
├── filter/             + Options.Eval (EvalCriteria); EvaluateEval(game, summary) as a final step
├── output/             + Options.StripEval (removes [%eval …] comments)
└── cmd/pgnx/           + analyze.go; filter/info/convert flag additions
```

Dependency direction holds: `chess` ← `uci`; `chess`,`pgn`,`replay`,`uci`
← `analysis`; `filter` imports `analysis` only for the `Summary` type and
`FromGame` (no engine code path); `cmd/pgnx` composes everything.

## Library API

```go
// uci
eng, err := uci.Start(ctx, uci.Config{
    Path:    "stockfish",
    Options: map[string]string{"Threads": "4", "Hash": "512"}, // after UCI_AnalyseMode=true, MultiPV
})
eng.ID()                                   // "Stockfish 17"
lines, err := eng.Search(ctx, uci.Position{FEN: fen, Moves: []string{"e2e4", "c7c5"}},
    uci.Limits{Depth: 20, MultiPV: 5, SearchMoves: nil})
// lines: []uci.Line{PV []string, Score, Depth, SelDepth, Nodes, Time}, best-first by Score.Compare
err = eng.NewGame(ctx); err = eng.Close()
pool, err := uci.NewPool(ctx, cfg, 4); e := pool.Acquire(); pool.Release(e)

// analysis
an := analysis.New(pool, analysis.Options{
    Limits: uci.Limits{Depth: 20}, MultiPV: 3,
    BookDepth: 0, Colors: analysis.Both, Cache: cache,
})
rep, err := an.Game(ctx, game, result)     // *analysis.Report; per ply: Index, Color, Played chess.Move,
                                           // PlayedScore, Lines []Line (best first), Loss int, Depth int
sum := rep.Summary()                       // {White, Black}: ACPL, Blunders, Top1Pct, Analysed plies
analysis.Annotate(game, result, rep, analysis.AnnotateOptions{
    Eval: true, NAGs: true, Thresholds: analysis.Thresholds{Inaccuracy: 50, Mistake: 100, Blunder: 300},
    BestLines: 1, OnlyWorse: false, SummaryTags: true,
})
sum, ok := analysis.FromGame(game)         // from WhiteACPL… tags, else from %eval comments
analysis.WriteXML(w, game, rep)            // uci-analyser layout
analysis.WriteJSONL(w, game, rep)
cache, err := analysis.OpenCache(path)     // nil cache = disabled
```

## CLI

```
pgnx analyze [output flags] [engine] [limits] [scope] [annotation] [file...]     (alias: analyse)

Engine:
    --engine PATH                 UCI engine program (default "stockfish", found on $PATH)
    --set-option NAME=VALUE       UCI option (repeatable; uci-analyser --setoption)
    --parallel N                  engine processes (default 1); output stays in input order
    --cache FILE                  JSONL evaluation cache, read on start and appended

Limits (exactly one):
    --depth N                     go depth N          (uci-analyser --searchdepth, default there 20)
    --nodes N                     go nodes N
    --movetime DURATION           go movetime (e.g. 500ms, 2s)
    --multipv N                   lines per position (default 3; uci-analyser --variations, default there 5)

Scope:
    --book-depth N                plies to skip at the start (default 0; a BookDepth tag overrides;
                                  ignored for games with a FEN tag)  (uci-analyser --bookdepth)
    --color white|black|both      whose moves to analyse (default both; --whiteonly/--blackonly)
    --skip-analyzed               pass through games that already carry %eval comments or summary tags

Annotation (PGN formats only):
    --eval                        append {[%eval …]} to every analysed move (and the final position)
    --nags                        assign ?! / ? / ?? from centipawn loss
    --inaccuracy N --mistake N --blunder N      thresholds in centipawns (50 / 100 / 300)
    --best-lines N                insert up to N better engine lines as variations (default 0)
    --only-worse                  annotate only plies whose loss reaches --inaccuracy
    --summary-tags                add WhiteACPL/BlackACPL/WhiteBlunders/BlackBlunders/WhiteTop1/BlackTop1/Annotator

Formats:
    -f san|source|lalg|…          convert's formats, annotated PGN
    -f xml                        uci-analyser XML (side-to-move scores; no PGN written)
    -f jsonl                      one JSON object per game (white-perspective scores)

pgnx filter … [--max-acpl [c:]N] [--min-blunders [c:]N] [--min-top1 [c:]PCT] [--engine PATH + limits]
pgnx info  …                      shows ACPL / blunders / top-1 when annotation is present
pgnx convert … --no-eval          strip [%eval …] comments
```

Usage errors (exit 1): no limit or more than one; `--multipv` < 1;
`--parallel` < 1; thresholds not increasing; `--best-lines`/`--eval`/
`--nags`/`--summary-tags` with `-f xml|jsonl`; `--min-top1` in `filter`
without `--engine` on a game that has no summary tags is **not** an error:
the game is simply unmatched (top-1 is unrecoverable from `%eval` alone),
and this is logged once per run. Engine start failure is exit 1.

With no annotation flag at all, `analyze -f san` is a usage error too:
the user asked for an engine run whose result would be discarded.

## Per-game pipeline (what the port must preserve)

For `analyze`, per game, in this order:

1. Parse, replay (Phase 1). Broken → skip or `--keep-broken` Source
   pass-through.
2. ECO classification if `--eco-file` (Phase 3; `analyze` carries the ECO
   flags like `convert`).
3. `--skip-analyzed`: if any main-line move has a `[%eval` comment or the
   game has a `WhiteACPL`/`BlackACPL` tag, emit unchanged.
4. Determine `startPly` = `BookDepth` tag (0..99, else warn and use the
   flag) or `--book-depth`; **0 when a `FEN` tag is present**.
5. `ucinewgame` (pool engine). For each main-line ply `i ≥ startPly` whose
   mover is selected by `--color`: build `uci.Position` from the game's FEN
   tag (or startpos) plus the UCI strings of moves `0..i-1` from
   `replay.Result.Moves[*].Move`; `Search` (cache first); if the played
   move's UCI string is not a line's first move, `Search` again with
   `SearchMoves=[played]` and merge; record `Ply{Lines, PlayedScore,
   Loss}` with `Loss = clamp(best) − clamp(played)` in the mover's
   perspective (0 when played is best, never negative).
6. If `--eval`, one more `Search` on the position after the last move for
   the terminal `%eval` (skipped when the terminal position is mate or
   stalemate: `#0`-style evals are written from `chess` status instead).
7. `Annotate` in this order so output is deterministic: NAG replacement →
   `%eval` comment appended → best-line RAVs appended after existing
   variations → summary tags appended (then `TagRegistry` orders them).
8. Render through `output` as `convert` does, or `WriteXML`/`WriteJSONL`.

For `filter` with eval criteria: the Phase 2/3 `Evaluate` chain runs
unchanged; only for games that pass does the CLI obtain a `Summary`
(`FromGame`, else analyse when `--engine` is given), then
`filter.EvaluateEval`. Engine work therefore never runs for games cheap
criteria already rejected.

## uci-analyser flag map (for MIGRATION.md)

| uci-analyser | pgnx |
|---|---|
| `analyse file` (after `pgn-extract -Wuci`) | `pgnx analyze -f xml file.pgn` (no pre-pass) |
| `--engine P` | `--engine P` |
| `--setoption N V` | `--set-option N=V` |
| `--searchdepth N` | `--depth N` |
| `--variations N` | `--multipv N` |
| `--bookdepth N` / `BookDepth` tag | `--book-depth N` / `BookDepth` tag |
| `--whiteonly` / `--blackonly` | `--color white` / `--color black` |
| `--annotate` | not ported; use `-f jsonl` |
| `--annotatePGN` | `--eval --best-lines 1` (real PGN) |
| default XML | `-f xml` |

## Conformance

uci-analyser output is the only oracle, and only for `-f xml`:

- Element and attribute layout, ordering, tag escaping (`&` → `&amp;`,
  `\"` → `&quot;`) match `analyse.cpp outputTag`/`showEvaluationsForMove`.
- Values are from the side to move, as the original; `mate N`; ` L`/` U`
  bound suffixes.
- **Documented divergences**: highest-depth-per-move collection instead of
  `depth == N` (so the early-stop case produces evaluations rather than an
  error); `depth` and `limit` attributes added; the `<!-- Processing … -->`
  comment is not emitted; `<moves>` content is pgnx's `-f uci` rendering
  (one line) rather than the input text echoed.

Engine numbers are not reproducible across engine versions, so there is no
byte-level golden against `example-output.xml`; the structural test below
is the check.

## Testing

1. **Unit, `uci`**: `info` line parsing table (cp, mate, bounds, missing
   fields, `pv` with promotions, Leela-style extra tokens); `Score.Compare`
   total order over mixed cp/mate including negative mates; handshake
   against `fakeuci` (id capture, options echoed, `uciok`/`readyok`); every
   limit kind; `searchmoves`; highest-depth collection when the stub stops
   early; cancellation sends `stop` then `quit`; engine death surfaces an
   error, not a hang; pool ordering under random per-game latency.
2. **Unit, `analysis`**: book depth from flag, from tag, out-of-range tag,
   FEN tag disables it; colour selection; played-move fallback path
   exercised (stub returns MultiPV without the played move); loss and clamp
   arithmetic incl. mates; `Summary` numbers on a hand-built report;
   `Annotate` on a game that already has comments/NAGs/variations (eval
   appended last, $1–$6 replaced, other NAGs kept, RAV after existing
   RAVs, SAN of PVs correct incl. castling/promotion/check suffix);
   `FormatEval`/`ParseEval` round trip; `FromGame` from tags and from
   comments; cache hit avoids a `Search` (counted on the stub).
3. **CLI**: `analyze` over `cmd/pgnx/testdata` games with the stub engine
   (committed expected PGN, XML and JSONL under `testdata/engine/`); exit
   codes for each usage error; `--parallel 3` output byte-identical to
   `--parallel 1`; `--skip-analyzed` idempotence (`analyze | analyze` is a
   no-op); `filter --max-acpl` on annotated input with and without
   `--engine`; `convert --no-eval` restores the pre-annotation file
   byte-for-byte for a game that had no comments.
4. **XML structure**: parse `~/code/uci-analyser/example-output.xml` and
   pgnx's XML for `example.pgn` with `encoding/xml` and compare the element
   tree shape and attribute names (not values).
5. **Manual smoke** (documented in the plan, not CI): Stockfish on the
   phase 3 golden inputs with `--depth 12 --parallel 4 --cache`, then
   lichess import of the annotated PGN to confirm `%eval` rendering.

## Non-goals

- Reproducing uci-analyser's evaluations numerically; engines moved on.
- Diagnostics wording; log lines from the engine's stderr are passed to
  `--log` verbatim and otherwise discarded.
- Windows process handling beyond what `os/exec` gives for free.
- Performance tuning beyond the pool and cache; a 20-ply search dominates
  everything pgnx does per game by orders of magnitude.
