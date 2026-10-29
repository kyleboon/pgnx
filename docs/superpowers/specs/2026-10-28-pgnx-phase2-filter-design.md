# pgnx: Go Port of pgn-extract — Phase 2 Design (`pgnx filter`)

**Date:** 2026-10-28
**Status:** Implemented (2026-10-29; see the plan for the commit range)
**Module:** `github.com/kyleboon/pgnx`
**Builds on:** [Phase 1 design](2026-08-07-pgnx-go-port-design.md) (status: complete)

## Context

Phase 1 delivered parsing (`pgn`), replay (`replay`), output (`output`), a
from-scratch chess core (`chess`), and `pgnx convert|validate|info`, all
byte-matched against the C pgn-extract oracle's golden corpus. Phase 2 adds
*game selection*: the ability to say which games in a collection should be
emitted, which was the original tool's main purpose ("extract").

In the C tool this is spread across `lists.c` (tag criteria), `taglines.c`
(the `-t` file grammar), `end.c` (material-balance matching and insufficient
material), `moves.c` (bounds, checkmate/stalemate), `hashing.c`
(repetition), and the `&&` chain in `grammar.c`'s `deal_with_game`, all
driven by `GlobalState`. The port collects it into one `filter` package with
a single `Options` struct and one evaluation function whose ordering mirrors
the C chain.

## Decisions

| Question | Decision |
|---|---|
| CLI shape | New verb `pgnx filter`, per the Phase 1 spec ("later phases add verbs, not flags"). It accepts every `convert` output flag plus selection flags, so `filter` with no criteria behaves exactly like `convert`. |
| Library shape | One new package `filter` (ports `lists.c`, `taglines.c`, `end.c`, the bounds/end-state checks from `moves.c`, repetition from `hashing.c`). No new third-party dependencies. |
| Where predicates that need a board live | Pure chess facts (insufficient material) go in `chess`. Per-position data the predicates need (halfmove clock, ply count, final board) is exposed by `replay`. `filter` consumes both and never touches the board directly. |
| Tag criteria syntax | The C `-t` file line grammar (`Name [op] "value"`) is the one grammar, used verbatim for `--tag-file` and for a repeatable `--tag EXPR` flag. A handful of `-T`-style shorthands (`--player`, `--white`, `--black`, `--date`, `--result`, `--eco`) are sugar that produce the same criteria. |
| Regex criteria (`=~`) | Implemented with Go's RE2 (`regexp`), not POSIX basic regex. Documented divergence in `CONFORMANCE.md`; the golden corpus has no `=~` case. |
| Malformed criteria files | The C tool silently stops reading at the first line that doesn't start with a tag name (`taglines.c:226`). pgnx returns a usage error (exit 1) instead. Documented divergence; diagnostics are not held to parity. |
| Evaluation order | Mirrors `grammar.c:1214-1229` + `apply.c` exactly where it is observable (which games go to `--output` vs `--unmatched`, what `--mark-matches` inserts, what `-Tr` sees relative to `--fix-result-tags`). See "Evaluation order" below. |
| Result-tag normalisation | Done inside `filter.Evaluate` (first step), mirroring `check_result` in `grammar.c:182`. Phase 1's `output` only fills an absent/empty Result at write time; the `"1/2"` and `"?"` cases are a Phase 1 `convert` gap to verify by differential run and either fix in `output` or document (Task 10). |
| Broken games | Same policy as `convert`: a game that fails replay is logged, dropped from both outputs, and makes the exit code 2. With `--keep-broken` it is evaluated against the partial replay result and emitted (in Source format) wherever a matching/unmatching game would go. |
| Marked-game comments | `--mark-matches TEXT` inserts `{ TEXT }` comments into the `pgn.Game` (after each qualifying mainline move) before output, mutating the parsed game, the same way the C tool does. The library documents this side effect on `Options.MarkMatches`. |

## Scope

### In (all have golden oracle files already vendored under `cmd/pgnx/testdata/golden/`)

- **Tag criteria** (`lists.c`, `taglines.c`): prefix matching (default), substring (`--tagsubstr`), soundex (`-S`), relational operators `< <= <> > >= =` with the C's numeric semantics, regex `=~`; pseudo-tags `Player`, `Elo`, `EloDiff`; `Date` with `a`/`b` prefixes and relational ops; `TimeControl` period extraction; `-Ta/-Tb/-Td/-Te/-Th/-Tp/-Tr/-Tt/-Tw` single-criterion shorthands (`-Te` only as a plain prefix match on an existing ECO tag; no classification).
- **Rating filters:** `--higherratedwinner`, `--lowerratedwinner`.
- **SetUp filters:** `--nosetuptags`, `--onlysetuptags`, including the `consistent_FEN_tags` side effect of adding `SetUp "1"` to a game with a FEN tag and no SetUp tag.
- **Bounds** (`moves.c:838`, `argsfile.c:1950`): `-b`, `-p` (`l`/`u`/`e` forms), `--minmoves/--maxmoves/--minply/--maxply`, with the C's ply-count definition (from the final board's move number, not a count of moves played).
- **End-state / game-property filters:** `--checkmate`, `--stalemate`, `--insufficient`, `--fifty`, `--seventyfive`, `--repetition`, `--repetition5`, `--underpromotion`, `--commented`, `--piececount N`.
- **Material-balance matching** (`end.c`): `-z FILE`, `-y FILE`, `--materialz`, `--materialy`, including the piece-set grammar (`KQRNBPL`, `* + - ? = # < <= > >=`, leading stability depth) and `--markmatches`/`--addmatchtag` interaction.
- **Match marking:** `--markmatches TEXT` for fifty/seventy-five, repetition, and material matches (the only Phase-2 matchers that insert comments).
- **Routing and selection:** `-n FILE` (unmatched games), `--suppressmatched`, `--selectonly`, `--skipmatching`, `--stopafter`, `--firstgame`, `--gamelimit`.
- **Result consistency:** `--nobadresults` (reject), `--fixresulttags` (rewrite from the final position).

### Out (deferred, with the phase that owns them)

- FEN/positional criteria: `-Tf`, `FEN`/`FENPattern` lines in `-t` files, `-x`, `-v`, `-H`, `--startply`, `--matchplylimit`, `--addlabeltag`, `-P`, `--wtm/--btm`, `--vanywhere` → **Phase 3**. `--startply` defaults to 1 in the C tool, so Phase 2's repetition/fifty/underpromotion checks simply scan from ply 1.
- ECO classification `-e`/`-E` → **Phase 3**.
- Duplicate detection `-D/-U/-d/-c/-Z`, `--fuzzydepth`, `--deletesamesetup` → **Phase 4**.
- `--odds` → needs null-move support (`--allownullmoves`), which `replay` does not have; revisit with null moves.
- Output-side transforms that are not selection: `--plycount`, `--totalplycount`, `--addhashcode`, `--dropply`, `--plylimit`, `--linenumbers`, `--splitvariants`, `--fencomments`, `--evaluation`, `--hashcomments`, `-F`, `--commentlines`, `--notags`, `--nochecks`, `--addelotags`, `--addfideidtags`, `--dropbefore`. These belong on `convert` and are a natural "Phase 2b: convert transforms" mini-phase; listed here so they are not forgotten.
- JSON output, argsfiles (`-A`), `-#` file splitting, `-r` check-only mode (covered by `pgnx validate`).

## Architecture

```
github.com/kyleboon/pgnx
├── chess/     + Board.InsufficientMaterial()          (end.c:841, C semantics exactly)
├── replay/    + Position.{HalfmoveClock,FullmoveNumber,SideToMove}
│              + Result.PlyCount()                      (apply.c:1777 definition)
│              + Result.FinalBoard()                    (reconstructed from the last FEN)
├── filter/    NEW — pure functions over (*pgn.Game, *replay.Result):
│     criteria.go   Op, Criterion, ParseCriterion, ReadCriteria   (taglines.c grammar)
│     tags.go       TagMatcher: generic/date/elo/elodiff/timecontrol (lists.c)
│     soundex.go    soundex()                                       (lists.c:222)
│     bounds.go     Bounds                                          (moves.c:838)
│     endstate.go   checkmate/stalemate/insufficient/fifty/repetition/underpromotion/commented
│     material.go   MaterialPattern, ParseMaterial, matching walk   (end.c)
│     result.go     nobadresults / fixresulttags                    (apply.c:1083-1189)
│     filter.go     Options, Evaluate (the ordered chain)           (grammar.c:1214)
│     select.go     Selector: selectonly/skipmatching/stopafter/firstgame/gamelimit
└── cmd/pgnx/  + filter.go   `pgnx filter`; shared output-flag helper factored out of convert.go
```

Dependency direction is preserved: `filter` imports `chess`, `pgn`, `replay`;
nothing imports `filter` except `cmd/pgnx`. `filter` never writes output; the
CLI composes `filter.Evaluate` + `filter.Selector` + two `output.Writer`s
(matched, unmatched).

## Library API

```go
// Tag criteria — one grammar for files and flags.
c, err := filter.ParseCriterion(`WhiteElo >= "2500"`)   // taglines.c line grammar
cs, err := filter.ReadCriteria(r)                        // % comments, blank lines, CRLF tolerated
m := &filter.TagMatcher{Substring: false, Soundex: false}
m.Add(c)                                                 // same tag ORs; different tags AND
m.AddPrefix("Player", "Fischer")                         // -Tp shorthand
ok := m.Match(game)

// Material balance.
p, err := filter.ParseMaterial("q*r*p*b2n2< q=r=p=b2<n2", filter.EitherColour)

// Everything, evaluated in the C tool's order.
opts := &filter.Options{
    Tags: m, Bounds: filter.Bounds{MinPly: 89}, Checkmate: true,
    Repetition: 3, MarkMatches: "REPEAT", Material: []*filter.MaterialPattern{p},
}
matched, err := filter.Evaluate(game, result, opts)
// err is filter.ErrBadResult when --nobadresults rejects the game;
// Evaluate may mutate game (SetUp tag, Result tag, mark comments) as documented.

// Selection bookkeeping (what to do with a matched/unmatched game).
sel := filter.NewSelector(filter.SelectorOptions{SelectOnly: ranges, StopAfter: 1})
d := sel.Decide(gameIndex, matched)   // d.Emit, d.ToUnmatched, d.Stop
```

## CLI

```
pgnx filter [output flags] [criteria] [file...]

Output (identical to convert, shared implementation):
    -f/--format, -o/--output, --append, -w/--line-width, --seven-tag-roster,
    --roster, --no-comments, --no-nags, --no-variations, --no-move-numbers,
    --no-results, --keep-broken, --nested-comments

Tag criteria (repeatable; same-tag ORed, different tags ANDed):
    --tag EXPR            EXPR in -t line grammar:  'White "Fischer"', 'WhiteElo >= "2500"'
    --tag-file FILE       -t FILE
    --player N --white N --black N --date D --result R --eco E     -Tp/-Tw/-Tb/-Td/-Tr/-Te
    --tag-substring       --tagsubstr          --soundex            -S
    --higher-rated-winner --lower-rated-winner
    --no-setup-tags       --only-setup-tags

Length (moves = full moves; ply = half moves; all inclusive):
    --min-moves N --max-moves N --moves N        -bl/-bu/-b(e)
    --min-ply N   --max-ply N   --ply N          -pl/-pu/-p(e)

Game properties:
    --checkmate --stalemate --insufficient --underpromotion --commented
    --fifty --seventy-five --repetition[=N]     (N defaults to 3; --repetition=5 is --repetition5)
    --piece-count N
    --material SPEC / --material-file FILE               -z   (either colour first)
    --white-material SPEC / --white-material-file FILE   -y   (first set is White)
    --mark-matches TEXT   --add-match-tag

Routing and selection:
    --unmatched FILE      -n FILE (also accepts "-" for stdout)
    --suppress-matched    --select-only R[,R...]   --skip-matching R[,R...]
    --stop-after N        --first-game N           --game-limit N

Result consistency:
    --no-bad-results      --fix-result-tags
```

Exit codes unchanged: 0 ok; 1 usage/flag/criteria-file error; 2 one or more
games broken (replay failure, or rejected by `--no-bad-results`). A run in
which no game matches exits 0, as in `convert`; "nothing matched" is not an
error.

Summary line: `"%d game(s) matched out of %d."` on the log writer unless
`--quiet` or `--suppress-matched` (the C tool suppresses it for the latter
too).

## Evaluation order (observable behaviour the port must preserve)

Per game, in the CLI:

1. Parse (`pgn.Scanner`). Note: pgnx's parser does **not** normalise the
   Result tag; the C tool does it in `check_result` (`grammar.c:182`) before
   any matching, so `filter.Evaluate` does it as its first step (see below).
2. Replay (`replay.Game`). Failure → log, count as broken, go nowhere
   (unless `--keep-broken`, in which case continue with the partial result).
3. `filter.Evaluate`, which internally runs, stopping at the first failure:
   0. Result-tag normalisation (`grammar.c:182`): `"1/2"` → `"1/2-1/2"`; an absent, empty, or `"?"` Result tag is filled from the terminating result token. Mutates the tag so `--result` and `--fix-result-tags` see what the C tool sees.
   1. FEN/SetUp consistency (adds `SetUp "1"` when FEN is present without it; rejects `SetUp "1"` without FEN).
   2. Tag criteria incl. rated-winner (`lists.c:888`). **Before** result fixing, so `--result` sees the original tag.
   3. SetUp-tag filter.
   4. `--no-bad-results` → `ErrBadResult` (treated like a broken game); `--fix-result-tags` → mutate.
   5. Bounds on `Result.PlyCount()`.
   6. Material / piece count (may insert mark comments).
   7. Checkmate; stalemate; insufficient material.
   8. Fifty / seventy-five; repetition; underpromotion (the first two may insert mark comments at every qualifying ply, matching `apply.c:1047-1064`).
   9. ECO prefix criteria (deliberately last, as in C, so a future `-e` can add the tag first).
   10. `--commented`.
4. `filter.Selector.Decide(index, matched)`:
   - `index < first-game` → never matched → unmatched output.
   - matched → increment match count; `--select-only` not in range, or `--skip-matching` in range → held back (emitted **nowhere**); otherwise emit unless `--suppress-matched`.
   - unmatched → `--unmatched` writer if set.
   - stop when `--select-only` ranges are exhausted, match count reaches `--stop-after`, or index reaches `--game-limit`. Games after the stop are not read.

The C tool skips replay when tag criteria fail and no `-n` is given. pgnx
always replays. That is an optimisation difference, not an observable one,
except that pgnx reports (and exits 2 for) broken games that the C tool would
never have examined. Documented in `CONFORMANCE.md`.

## Testing

1. **Unit tests** in `filter`: criterion grammar table (every operator, quoting, CRLF, `%` comments, the "stops at non-tag line" case), per-tag semantics tables lifted from `lists.c` (date encoding, Elo ranges, EloDiff, TimeControl formats, order-dependent date mixing), soundex vectors (Nimzovich/Nimsowitsch, Tal/Talj, Yusupov/Jusupov, Janosevic≠Nimzovich), bounds conversion table, each end-state predicate on hand-built games, material grammar and matching on fischer.pgn extracts, selector state machine.
2. **Golden conformance**: new rows in `cmd/pgnx/conformance_test.go` for every Makefile target in scope (listed in the plan), keeping the "C command as a comment" convention. Two-output targets (`test-n`, `test-suppressmatched`) get dedicated test functions like the existing append/multi-file ones. The table gains an optional "expect exit 2" field for `test-nobadresults`.
3. **Differential**: `scripts/difftest.sh` gains a way to pass extra flags to both binaries (`PGNX_FLAGS`/`C_FLAGS` env vars) so filter runs can be diffed over large real-world files; run a sample of criteria over the golden corpus and at least one large file before closing the phase.

## Non-goals (Phase 2)

- Byte parity for diagnostics (per Phase 1 policy).
- POSIX regex parity for `=~`.
- Performance work beyond keeping streaming/constant-memory behaviour (repetition counting is per game and bounded by game length; no cross-game state except the selector counters).
