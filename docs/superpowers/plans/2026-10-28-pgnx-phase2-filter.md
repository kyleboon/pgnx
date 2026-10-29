# pgnx Phase 2 Implementation Plan — `pgnx filter`

> **Status: COMPLETE** (verified 2026-10-29; commits `32ca42a`..`HEAD` on `main`). Spec: [`docs/superpowers/specs/2026-10-28-pgnx-phase2-filter-design.md`](../specs/2026-10-28-pgnx-phase2-filter-design.md).
> All 36 in-scope golden targets byte-match the oracle; a 33-combination differential sweep over the golden inputs and a Chess.com export is clean. Two Phase 1 `convert` divergences surfaced by the sweep (run-wide unknown-tag order, Result-tag normalisation) were fixed; see CONFORMANCE.md.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `pgnx filter` selects games from PGN collections by tag criteria, length, end state, material balance, and result consistency, routes matched/unmatched games to separate outputs, and byte-matches the C pgn-extract oracle on every in-scope golden target.

**Architecture:** A new `filter` package (pure predicates over `*pgn.Game` + `*replay.Result`, plus a `Selector` state machine), small additive changes to `chess` and `replay` to expose the facts those predicates need, and a `filter` subcommand that composes them with the existing `output.Writer`. The C `&&` chain in `grammar.c:1214-1229` is the ordering oracle.

**Tech Stack:** Go 1.23+, `spf13/cobra` (CLI only), standard library (`regexp` for `=~`). Reference C code: `~/code/pgn-extract` (v26-06). All line numbers below refer to that tree.

## Global Constraints

Carried over from Phase 1 unchanged:

- Module `github.com/kyleboon/pgnx`; GPL-3.0; cobra only under `cmd/`; library packages never `os.Exit`, never panic on user input, never write to stdout/stderr.
- Every task: `go build ./... && go test ./... && go vet ./... && gofmt -l .` clean before committing. Conventional commit messages.
- The C source is read-only reference; cite `file.c:line` in comments only where behaviour is surprising.
- Oracle precedence when ambiguous: golden `outfiles/` → C source → never guess.
- `CONFORMANCE.md` first: check it before "fixing" any output difference; document new intentional divergences there.
- **Streaming:** no cross-game state beyond the selector's counters. Repetition/fifty tracking is per game and discarded.
- **Dependency direction:** `filter` imports `chess`, `pgn`, `replay`; only `cmd/pgnx` imports `filter`.

## Oracle target → pgnx invocation map

Every Makefile target in scope, with the pgnx command that must byte-match it. Task 9 turns this table into conformance rows. (All input and oracle files are already vendored under `cmd/pgnx/testdata/golden/`; verified 2026-10-28.)

| C target | C command (on `infiles/`) | pgnx `filter` args | oracle |
|---|---|---|---|
| test-bl45 | `-bl45 fischer.pgn` | `--min-moves 45` | test-bl45-out.pgn |
| test-bu45 | `-bu45 fischer.pgn` | `--max-moves 45` | test-bu45-out.pgn |
| test-bu30 | `-b30 fischer.pgn` | `--moves 30` | test-b30-out.pgn |
| test-pl90 | `-pl90 fischer.pgn` | `--min-ply 90` | test-pl90-out.pgn |
| test-pu90 | `-pu90 fischer.pgn` | `--max-ply 90` | test-pu90-out.pgn |
| test-pu60 | `-p60 fischer.pgn` | `--ply 60` | test-p60-out.pgn |
| test-TT (d) | `-Td1970 fischer.pgn` | `--date 1970` | test-TTd-out.pgn |
| test-TT (dd) | `-Td1960 -Td1962 fischer.pgn` | `--date 1960 --date 1962` | test-TTdd-out.pgn |
| test-TT (b) | `-TbPetrosian fischer.pgn` | `--black Petrosian` | test-TTb-out.pgn |
| test-TT (p) | `-TpPetrosian fischer.pgn` | `--player Petrosian` | test-TTp-out.pgn |
| test-TT (r) | `-Tr0-1 fischer.pgn` | `--result 0-1` | test-TTr-out.pgn |
| test-TT (w) | `-TwFischer fischer.pgn` | `--white Fischer` | test-TTw-out.pgn |
| test-TT (a) | `-TaBarnes test-Ta.pgn` | `--tag 'Annotator "Barnes"'` | test-TTa-out.pgn |
| test-t | `-ttaglist.txt test-t.pgn` | `--tag-file taglist.txt` | test-t-out.pgn |
| test-reltags | `-treltags.txt bots.pgn` (file has CRLF endings) | `--tag-file reltags.txt` | test-reltags-out.pgn |
| test-elodiff | `-t test-elodiff.txt test-seventyfive.pgn` | `--tag-file test-elodiff.txt` | test-elodiff-out.pgn |
| test-SS | `-S -TpPetrosian test-ucS.pgn` | `--soundex --player Petrosian` | test-SS-out.pgn |
| test-ratedwins | `--higherratedwinner bots.pgn` / `--lowerratedwinner bots.pgn` | `--higher-rated-winner` / `--lower-rated-winner` | test-higherratedwins-out.pgn / test-lowerratedwins-out.pgn |
| test-setup | `--nosetuptags test-setup-in.pgn` / `--onlysetuptags ...` | `--no-setup-tags` / `--only-setup-tags` | test-nosetup-out.pgn / test-onlysetup-out.pgn |
| test-checkmate | `--checkmate test-checkmate.pgn` | `--checkmate` | test-checkmate-out.pgn |
| test-stalemate | `--stalemate test-stalemate.pgn` | `--stalemate` | test-stalemate-out.pgn |
| test-fifty | `--fifty --markmatches FIFTY test-fifty.pgn` | `--fifty --mark-matches FIFTY` | test-fifty-out.pgn |
| test-seventyfive | `--seventyfive --markmatches SEVENTYFIVE test-seventyfive.pgn` | `--seventy-five --mark-matches SEVENTYFIVE` | test-seventyfive-out.pgn |
| test-repetition | `--repetition --markmatches REPEAT test-repetition.pgn` | `--repetition --mark-matches REPEAT` | test-repetition-out.pgn |
| test-promotion (2nd) | `--underpromotion test-promotion-in.pgn` | `--underpromotion` | test-underpromotion-out.pgn |
| test-z | `-zzmatch.txt --markmatches MATCH fischer.pgn`; also `--materialz "q*r*p*b2n2< q=r=p=b2<n2"` | `--material-file zmatch.txt --mark-matches MATCH`; `--material 'q*r*p*b2n2< q=r=p=b2<n2' --mark-matches MATCH` | test-z-out.pgn |
| test-y | `-yymatch.txt --markmatches MATCH fischer.pgn`; `--materialy "..."` | `--white-material-file ymatch.txt --mark-matches MATCH` | test-y-out.pgn |
| test-selectonly | `-TpPetrosian --selectonly 2,3 test-selectonly.pgn` | `--player Petrosian --select-only 2,3` | test-selectonly-out.pgn |
| test-skipmatching | `-TpKeres --skipmatching 1,2 fischer.pgn` | `--player Keres --skip-matching 1,2` | test-skipmatching-out.pgn |
| test-stopafter | `-TpPetrosian --stopafter 1 test-stopafter.pgn` | `--player Petrosian --stop-after 1` | test-stopafter-out.pgn |
| test-nobadresults | `--nobadresults test-nobadresults.pgn` (exit 2 expected from pgnx) | `--no-bad-results` | test-nobadresults-out.pgn |
| test-fixresulttags | `--fixresulttags test-fixresulttags-in.pgn` | `--fix-result-tags` | test-fixresulttags-out.pgn |
| test-n | `-TpFischer -o matched -n unmatched petrosian.pgn` (two outputs) | `--player Fischer -o M --unmatched U` | test-n-matched.pgn + test-n-unmatched.pgn |
| test-suppressmatched | `-TpPetrosian --suppressmatched -nnon-petrosian.pgn fischer.pgn` | `--player Petrosian --suppress-matched --unmatched U` (stdout must be empty) | non-petrosian.pgn |

Excluded (later phases, see spec): test-tFEN, test-Te, test-x/-v/-y-positional, test-markmatches (uses `-x`), test-odds, test-matchplylimit, test-startply, test-FENPattern, dedup targets, ECO targets, output-transform targets.

---

### Task 1: `chess` + `replay` additions the predicates need

**Files:**
- Modify: `chess/status.go`, `replay/replay.go`
- Test: `chess/status_test.go`, `replay/replay_test.go`

**Interfaces:**
- Consumes: existing `chess.Board`, `replay.Result`.
- Produces:
  - `func (b *Board) InsufficientMaterial() bool` — exact port of `end.c:841-920` (`insufficient_material` + `opposite_colour_bishops`): no pawns/rooks/queens on either side; at most one bishop per side; no side has both bishop and knight; a side with ≥2 knights fails if the other side has any minor piece; two lone bishops fail if on opposite-colour squares. Note the C rule is deliberately looser than FIDE (KNN v K is "insufficient"); port it as-is, do not "correct" it.
  - `replay.Position` gains `HalfmoveClock int`, `FullmoveNumber int`, `SideToMove chess.Color` (set in `positionOf`).
  - `func (r *Result) PlyCount() int` — the C definition at `apply.c:1777-1784`: from the **final** position, `2*(FullmoveNumber-1)+1` if Black to move, else `2*(FullmoveNumber-1)`. For games starting from a FEN this is the absolute half-move number, not moves played; that is what the C bounds test uses (`moves.c:838`), so keep it.
  - `func (r *Result) FinalBoard() (*chess.Board, error)` — `chess.FromFEN(r.Positions[len-1].FEN)`. (Reconstructing avoids storing a Board per ply; the Writer already pays for the FEN string.)
  - `replay.MoveInfo` already has `Move.Promotion`; confirm it is populated for promotions and add a test that an underpromotion replay reports `Promotion == chess.Knight`.

- [x] **Step 1: Write failing tests.** `InsufficientMaterial` table: `K v K`, `KB v K`, `KN v K`, `KNN v K` (true, C quirk), `KB v KB` same colour (true) / opposite colour (false), `KBN v K` (false), `KNN v KN` (false), `KP v K` (false), `KR v K` (false). `PlyCount`: a 3-ply game → 3; fools mate (4 ply) → 4; empty game → 0; game from FEN `... b - - 0 40` with one move → 80. `FinalBoard` round-trips `Positions[last].FEN`.
- [x] **Step 2: Implement.** Pure functions; no API removed.
- [x] **Step 3: `go test ./chess ./replay ./output ./cmd/...` all green** (output's EPD/FEN tests exercise `Position`; they must not change).
- [x] **Step 4: Commit** — `feat: insufficient material, per-position clocks and ply count for filtering`

---

### Task 2: `filter` — criterion grammar and file reader

**Files:**
- Create: `filter/criteria.go`, `filter/doc.go`
- Test: `filter/criteria_test.go`

**Interfaces:**
- Produces:
  - `type Op int` — `OpPrefix` (no operator), `OpLT`, `OpLE`, `OpNE`, `OpGT`, `OpGE`, `OpEQ`, `OpRegex`.
  - `type Criterion struct { Tag string; Op Op; Value string; re *regexp.Regexp }`.
  - `func ParseCriterion(line string) (Criterion, error)` — the `taglines.c:107-229` grammar: `Tag` is `[A-Za-z0-9_]+` (case-sensitive, `lex.c:758`); optional whitespace; optional operator from `< <= <> > >= = =~`; optional whitespace; a double-quoted value with backslash escapes (`lex.c:356`); anything after the closing quote ignored. Errors: no leading tag name; missing opening/closing quote; invalid regex for `=~`.
  - `func ReadCriteria(r io.Reader) ([]Criterion, error)` — line-oriented; skips blank lines and lines whose column 0 is `%` (`lines.c:70`); strips a trailing `\r` (reltags.txt is CRLF); returns `ParseCriterion`'s error wrapped with the line number. **Divergence (documented in Task 10):** C silently stops at the first non-tag line; pgnx errors.
  - Reserved names recognised but rejected in Phase 2 with a clear error: `FEN`, `FENPattern`, `FENPatternI` ("positional criteria are not supported yet").
  - Pseudo-tags accepted as ordinary names: `Player`, `Elo`, `EloDiff` (`lex.c:132-147`).

- [x] **Step 1: Write failing tests** (table): `White "Fischer"` → prefix; `WhiteElo >= "800"`; `BlackTitle <> "BOT"`; `Date<"1962"` (no spaces); `Event =~ "^World.*"` compiles; `TimeControl >= "180"\r` (CRLF); `% comment` skipped; blank skipped; `[Event "x"]` → error naming line 1; `White Fischer` (unquoted) → error; `Difficulty "5"` (unknown tag) → accepted; `FEN "..."` → unsupported error; escaped quote inside value preserved.
- [x] **Step 2: Implement.**
- [x] **Step 3: Test, vet, commit** — `feat(filter): tag criterion grammar and criteria file reader`

---

### Task 3: `filter` — tag matching (`lists.c`) + soundex

**Files:**
- Create: `filter/tags.go`, `filter/soundex.go`
- Test: `filter/tags_test.go`, `filter/soundex_test.go`

**Interfaces:**
- Produces:
  - `type TagMatcher struct { Substring bool; Soundex bool; /* criteria grouped by tag, insertion order kept */ }`
  - `func (m *TagMatcher) Add(c Criterion)`; `func (m *TagMatcher) AddPrefix(tag, value string)` (the `-T` shorthand); `func (m *TagMatcher) Empty() bool`.
  - `func (m *TagMatcher) Match(g *pgn.Game) bool` — port of `check_tag_details_not_ECO` (`lists.c:888-1025`) **excluding** ECO, plus `func (m *TagMatcher) MatchECO(g) bool` for the ECO group (`lists.c:1027`, called last by Task 6).
  - `func (m *TagMatcher) RatedWinner(g *pgn.Game, higher bool) bool` — `lists.c:1071`: needs WhiteElo, BlackElo, Result; Result must be exactly `1-0` or `0-1`; `diff = winner-loser`; higher ⇒ `diff > 0`, lower ⇒ `diff < 0`; equal ratings fail both.
  - `func soundex(s string) string` — exact port of `lists.c:222-268` (mapping `"01230120002455012622011202"`, leading `J`/`Y` → `7`, drops non-alpha, collapses repeated source letters and repeated codes, max 50). Applied to White/Black/Player/Event/Site/Annotator criteria **and** game values when `Soundex` is set (`lists.c:270-293`).

  Semantics to port exactly (all cited in the spec's inventory; re-read the C before coding each):
  - **Generic `check_list` (`lists.c:707`)**: pass 1 — any `OpPrefix` criterion matching (prefix via `strings.HasPrefix`; substring via `strings.Contains` when `Substring`) ⇒ true. Pass 2 — any `OpRegex` matching ⇒ true. Pass 3 — only if a `<>` criterion exists *or* the game value is numeric (`^[+-]?[0-9.]+$`): AND over all relational/`<>` criteria; relational ops skipped (leave result unchanged) for non-numeric values; `=` is numeric equality; `<>` on a non-numeric value means "not a prefix/substring match".
  - **Missing tag**: fail unless every criterion for that tag is `<>` (`all_negative_matches`, `lists.c:867`). ECO has no such exception.
  - **Player** (`lists.c:915-931`): White against the Player list, then Black; both missing ⇒ fail. Reproduce the `Elo` quirk at `lists.c:933` (an `Elo` group assigns rather than ANDs, so it can override a failed Player match) — write a test pinning it and a comment citing the line.
  - **Date (`check_date`, `lists.c:447`)**: year via leading unsigned int (fail if unparsable, e.g. `????.??.??`); month/day default 1; encoded `Y*10000+M*100+D`; criterion prefix `b` ⇒ before, `a` ⇒ after (strict), prefix stripped; relational ops require `100 < year < 3000`; plain criteria are prefix matches (`-Td1970` = that year only). Order-dependent mixing: relational at index 0 sets, later relational ANDs, prefix entries evaluated only when `index == 0 || !wanted`.
  - **Elo / WhiteElo / BlackElo (`check_elo`, `lists.c:611`)**: unsigned parse (fail otherwise); if any relational criterion exists, AND them all and ignore prefix criteria; else OR prefix criteria. `Elo` pseudo-tag: White tested against all criteria, then Black.
  - **EloDiff (`lists.c:657`)**: both Elo tags required; `abs(w-b)`; AND relational; ignore prefix.
  - **TimeControl (`lists.c:522-610`)**: first control before `:`; forms `N+inc`→N, `*N`→N, `moves/secs`→secs, digits→N; `""`, `?`, `-` ⇒ false; **criteria ORed including relational** (unlike Elo).
  - Across tags: AND, first failure stops.

- [x] **Step 1: Write failing tests.** Hand-built `pgn.Game`s with tag sets; a table per rule above, including: `-Tr1` matches both `1-0` and `1/2-1/2`; `Fischer` matches `Fischer, Robert J.` but not `Robert Fischer` unless `Substring`; date `1960`+`1962` ORs; `b1962` vs `1962.05.05`; Elo range on one player; EloDiff ≤ 50; reltags.txt criteria vs. a bot game and a human game; rated-winner on draws/equal ratings; soundex pairs Nimzovich/Nimsowitsch, Tal/Talj, Yusupov/Jusupov equal, Janosevic/Nimzovich different.
- [x] **Step 2: Implement.**
- [x] **Step 3: Test, vet, commit** — `feat(filter): tag criteria matching with date, Elo, time-control and soundex rules`

---

### Task 4: `filter` — bounds and end-state predicates

**Files:**
- Create: `filter/bounds.go`, `filter/endstate.go`
- Test: `filter/bounds_test.go`, `filter/endstate_test.go`

**Interfaces:**
- Produces:
  - `type Bounds struct { MinPly, MaxPly int }` with `MaxPly == 0` meaning unbounded (C default 10000). Helpers mirroring `set_move_bounds` (`argsfile.c:1950`): `func (b *Bounds) SetMinMoves(n int)` → `MinPly = 2*(n-1)+1`; `SetMaxMoves(n)` → `MaxPly = 2n`; `SetMoves(n)` both; `SetMinPly/SetMaxPly/SetPly`. `func (b Bounds) Match(plyCount int) bool` — inclusive both ends (`moves.c:838`). Reject `n == 0` for the moves forms with an error at the CLI layer (C underflows to "nothing matches"; pgnx makes that a usage error — note in CONFORMANCE).
  - `func matchCheckmate(r *replay.Result) bool` — any mainline `MoveInfo.IsMate` (`moves.c:686`; effectively the last move).
  - `func matchStalemate(r) bool` — last mainline move not check (or zero moves) and `FinalBoard().IsStalemate()` (`moves.c:712`).
  - `func matchInsufficient(r) bool` — `FinalBoard().InsufficientMaterial()`.
  - `func matchNMoveRule(g *pgn.Game, r, n int, mark string) bool` — after each mainline ply `i`, `Positions[i+1].HalfmoveClock >= 2*n`; when true and `mark != ""`, append `mark` to `g.Moves[i].Comments` (every qualifying ply, `apply.c:1054-1064`); return whether any ply qualified.
  - `func matchRepetition(g, r, n int, mark string) bool` — count `Positions[].Zobrist` occurrences including the start position (`apply_move_list:1761`); after ply `i`, if the count of `Positions[i+1]` so far `>= n`, mark `g.Moves[i]` and set matched (`apply.c:1047`, `hashing.c:112`). Polyglot Zobrist already folds in side-to-move, castling, and capturable-only en passant, which is exactly `hashing.c:163`'s position identity; cite that in a comment.
  - `func matchUnderpromotion(r) bool` — any mainline `Move.Promotion` not `NoPiece` and not `Queen` (`apply.c:1066`).
  - `func matchCommented(g) bool` — prefix comment, or any comment on any move at any variation depth (`grammar.c:238`).

- [x] **Step 1: Write failing tests.** Bounds conversion table (`bl45`→89, `bu45`→90, `b30`→59..60, `pl90`, `pu90`, `p60`); end-state predicates on hand-written games (fools mate; a stalemate; `K v K`; a 50-move clock game built from a FEN with clock 98 plus two quiet moves ⇒ marks exactly on the plies where clock ≥ 100; a threefold repetition `Nf3 Nf6 Ng1 Ng8 ...` ⇒ marks on the plies where the count reaches 3, and a `--repetition=5` variant; underpromotion `e8=N`; comments nested in a variation).
- [x] **Step 2: Implement.**
- [x] **Step 3: Test, vet, commit** — `feat(filter): length bounds and end-state predicates with match marking`

---

### Task 5: `filter` — material-balance matching and piece count (`end.c`)

**Files:**
- Create: `filter/material.go`
- Test: `filter/material_test.go`

**Interfaces:**
- Produces:
  - `type Colours int` — `EitherColour` (`-z`), `WhiteFirst` (`-y`).
  - `type MaterialPattern struct { /* num[2][piece], occurs[2][piece], minor counts, moveDepth, bothColours */ }`
  - `func ParseMaterial(line string, c Colours) (*MaterialPattern, error)` — `decompose_line` (`end.c:297`) + `extract_piece_information` (`end.c:222`) + `extract_combination` (`end.c:121`): optional leading integer = stability depth in half-moves (default 2); two whitespace-separated piece sets (second omitted ⇒ lone king); pieces `KQRNBP` case-insensitive, `L` = any minor; per-piece optional digit then optional occurrence suffix `*`,`+`,`-`,`?`,`=`,`#`,`<`,`<=`,`>`,`>=`; trailing text ignored.
  - `func ReadMaterialFile(r io.Reader, c Colours) ([]*MaterialPattern, error)` — one pattern per non-blank line (`build_endings`, `end.c:817`).
  - `func matchMaterial(g *pgn.Game, r *replay.Result, patterns []*MaterialPattern, mark string, addTag bool) bool` — `look_for_material_match` (`end.c:550`): walk the mainline tracking piece counts per side (start position, then after each capture/promotion), test each pattern via `material_match`/`piece_set_match`/`piece_match` (`end.c:371-548`) for both colour assignments when `EitherColour`, require the relationship to hold for `moveDepth` consecutive half-moves, stop at first match. On match: append `{ mark }` after the matching move, or to `g.PrefixComments` when the match is at the start position (`end.c:924`); when `addTag`, add tag `MaterialMatch "White"`/`"Black"` (`--addmatchtag`).
  - `func matchPieceCount(r, n int) bool` — `look_for_number_of_pieces` (`end.c:712`): some mainline position (start, or after a capture) has exactly `n` pieces including kings.

  Use `chess.Board` piece counts from `FinalBoard()`-style reconstruction only where needed; the walk should derive counts from `MoveInfo.Captured` and `Move.Promotion` incrementally from the start position to stay O(plies).

- [x] **Step 1: Write failing tests.** Grammar table: `q*r*p*b2n2< q=r=p=b2<n2` (zmatch.txt) parses to the expected occurs/num per piece; `3 kr k` depth 3; `KQ` (lone king opponent); `L2` minor pieces; bad piece letter ⇒ error. Matching: a hand-built game where a queen trade produces `R v R` and a pattern `r r` matches after the stability depth, with the mark comment on the right move; start-position match goes to the prefix comment; `WhiteFirst` vs `EitherColour` on a colour-asymmetric pattern. Piece count: a game reaching exactly 7 pieces.
- [x] **Step 2: Implement.**
- [x] **Step 3: Test, vet, commit** — `feat(filter): material balance patterns and piece-count matching`

---

### Task 6: `filter` — result consistency, FEN/SetUp consistency, and `Evaluate`

**Files:**
- Create: `filter/result.go`, `filter/filter.go`
- Test: `filter/result_test.go`, `filter/filter_test.go`

**Interfaces:**
- Produces:
  - `var ErrBadResult = errors.New("filter: result tag disagrees with the game's result")`
  - `type SetupFilter int` — `AnySetup`, `OnlySetup`, `NoSetup`.
  - `type Options struct { Tags *TagMatcher; HigherRatedWinner, LowerRatedWinner bool; Setup SetupFilter; NoBadResults, FixResultTags bool; Bounds Bounds; Material []*MaterialPattern; AddMatchTag bool; PieceCount int; Checkmate, Stalemate, Insufficient bool; NMoveRule int /* 0, 50, 75 */; Repetition int /* 0, 3, 5, … */; Underpromotion, Commented bool; MarkMatches string }`
  - `func Evaluate(g *pgn.Game, r *replay.Result, o *Options) (bool, error)` — the ordered chain from the spec ("Evaluation order"), stopping at first failure. Documented side effects on `g`: may add `SetUp "1"`, may rewrite the `Result` tag and `g.Result` (`FixResultTags`), may append mark comments and a `MaterialMatch` tag.
  - `func normaliseResult(g *pgn.Game)` — `check_result` (`grammar.c:182-203`): a Result tag equal to `"1/2"` becomes `"1/2-1/2"`; if the game has a terminating result token and the tag is absent, empty, or `"?"`, set the tag to the token. First step of `Evaluate`, before tag matching, so `--result 1/2` and `--fix-result-tags` see the normalised value (the C tool runs it in the parser, ahead of everything).
  - `func consistentFENTags(g *pgn.Game) bool` — `grammar.c` `consistent_FEN_tags`: FEN present and valid with no SetUp ⇒ add `SetUp "1"`; `SetUp "1"` without FEN ⇒ false; unparsable FEN ⇒ false. (Check how `output` orders a tag appended at the end against `test-onlysetup-out.pgn`; the Writer's canonical roster order should place `SetUp` correctly — if not, that is an output bug to fix with the oracle as justification.)
  - `func checkResult(g, r, fix, reject bool) error` — `apply.c:1083-1189`, evaluated only when the game has ≥1 mainline move:
    - `fix`: (a) final move is mate ⇒ Result `1-0`/`0-1` by mating side; (b) else if Result doesn't start with `1/2` and final position is stalemate or insufficient material ⇒ `1/2-1/2`; (c) then if tag and terminating result differ and either is `*` ⇒ tag := terminating result (even if `*`). Both `g.Result` and the tag are overwritten in (a)/(b).
    - `reject`: if tag ≠ terminating result and terminating result ≠ `*` ⇒ `ErrBadResult`. Mate/stalemate disagreement with the tag only warns in C; do not reject for it.

- [x] **Step 1: Write failing tests.** Normalisation table (`1/2`, `?`, empty, absent, already-normal); result fixing table from `test-fixresulttags-in.pgn` cases (mate with `*`, stalemate with `1-0`, tag `*` vs token `1-0`, tag `1-0` vs token `*`); rejection table; an `Evaluate` ordering test proving `--result` sees the pre-fix tag; a `NoSetup` test where a FEN game without SetUp is excluded; a short-circuit test (tag failure ⇒ no mark comments added).
- [x] **Step 2: Implement.**
- [x] **Step 3: Test, vet, commit** — `feat(filter): result-tag consistency and ordered Evaluate`

---

### Task 7: `filter` — `Selector` (select-only / skip-matching / stop-after / first-game / game-limit)

**Files:**
- Create: `filter/select.go`
- Test: `filter/select_test.go`

**Interfaces:**
- Produces:
  - `type Range struct { First, Last int }`; `func ParseRanges(s string) ([]Range, error)` — `extract_game_number_list` (`argsfile.c:1879`): comma-separated `N` or `N1:N2`, all ≥ 1, strictly ascending; error otherwise.
  - `type SelectorOptions struct { SelectOnly, SkipMatching []Range; StopAfter, FirstGame, GameLimit int }`
  - `type Decision struct { Emit, ToUnmatched, Stop bool }`
  - `func NewSelector(o SelectorOptions) *Selector`; `func (s *Selector) Decide(gameIndex int, matched bool) Decision`; `func (s *Selector) Matched() int`.
  - Semantics (`grammar.c:1290-1345`, `grammar.c:367`): `gameIndex < FirstGame` ⇒ not matched ⇒ `ToUnmatched`. Matched ⇒ `matched++`; `SelectOnly` set and `matched` not in current range ⇒ held back (neither output); `SkipMatching` and in range ⇒ held back; else `Emit`. Unmatched ⇒ `ToUnmatched`. `Stop` when `SelectOnly` ranges exhausted, `matched == StopAfter`, or `gameIndex >= GameLimit`. (C evaluates the stop after parsing the *next* game and drops it; pgnx stops before reading it — same output, different "processed" count; diagnostics are not held to parity.)

- [x] **Step 1: Write failing tests.** Range parsing (valid, descending ⇒ error, `0` ⇒ error, `2:4,7`); a scripted sequence of (index, matched) pairs for each option and for `--select-only 2,3` reproducing test-selectonly (matches 1..4 ⇒ emit 2 and 3 only, stop after 3).
- [x] **Step 2: Implement.**
- [x] **Step 3: Test, vet, commit** — `feat(filter): game selection bookkeeping`

---

### Task 8: CLI — `pgnx filter`

**Files:**
- Modify: `cmd/pgnx/convert.go` (factor flag registration/parsing into `addOutputFlags(cmd)` + `outputOptions(cmd)`), `cmd/pgnx/games.go` (early-stop support), `cmd/pgnx/root.go` (register)
- Create: `cmd/pgnx/filter.go`
- Test: `cmd/pgnx/filter_test.go`, keep `convert_test.go` green

**Interfaces:**
- `forEachGame`/`scanGames`: a callback returning the new sentinel `errStop` ends iteration cleanly (not logged, not counted as a failure). Document it on `forEachGame`.
- `pgnx filter [file...]` flags exactly as listed in the spec's CLI section. Flag → `filter.Options` mapping lives in `filterOptions(cmd) (*filter.Options, filter.SelectorOptions, error)`; validation errors (bad range, bad criteria file, `--checkmate` with `--stalemate`/`--insufficient`, `--fifty` with `--seventy-five`, `--higher-` with `--lower-rated-winner`, `--no-setup-tags` with `--only-setup-tags`, moves bound of 0) are usage errors (exit 1).
- `--repetition` is an `Int` flag with `NoOptDefVal = "3"` so both `--repetition` and `--repetition=5` work.
- `--unmatched FILE`: `-` means stdout. Opened with the same truncate/append rules as `--output`.
- Per-game loop: replay → (broken: log, `--keep-broken` ⇒ continue with partial result, else `return err`) → `filter.Evaluate` (`ErrBadResult` ⇒ log + treat as broken) → `Selector.Decide` → write to matched writer / unmatched writer / neither; `Stop` ⇒ `return errStop`. Writers are `output.NewWriter` with the shared options; broken games under `--keep-broken` use a Source-format writer as `convert` does.
- Summary: `"%d game(s) matched out of %d."` to the log writer unless `--quiet` or `--suppress-matched`. Exit 2 iff any broken/rejected game or I/O error.

- [x] **Step 1: Write failing tests** (reuse `runCLI` from `convert_test.go`): no criteria ⇒ output identical to `convert`; `--player` routes to `-o` and `--unmatched`; `--suppress-matched` leaves stdout empty; `--stop-after 1` stops reading (feed a second, *unparsable* game after the first match and assert exit 0); conflicting flags ⇒ exit 1; `--no-bad-results` ⇒ exit 2 with the bad game logged; `--tag-file` missing ⇒ exit 1.
- [x] **Step 2: Implement.** Keep `convert.go` behaviour byte-identical (its conformance rows are the regression check for the refactor).
- [x] **Step 3: Test, vet, commit** — `feat: pgnx filter command`

---

### Task 9: Golden conformance rows

**Files:**
- Modify: `cmd/pgnx/conformance_test.go`

- [x] **Step 1:** Add a row per line of the "Oracle target → pgnx invocation" table above, keeping the C command as a comment on each row. Extend the row struct with `wantProblems bool` (exit 2 accepted) for test-nobadresults, and adjust the runner so a `wantProblems` row tolerates `errProblems` but still fatals on any other error. Rows referencing criteria/material files pass `filepath.Join("testdata","golden","infiles", …)` — add a `files []string`-style substitution or just build the arg in the row with the relative path (the test runs with `cmd/pgnx` as cwd).
- [x] **Step 2:** Add `TestGoldenConformanceUnmatched` (test-n: two output files compared) and `TestGoldenConformanceSuppressMatched` (stdout empty, unmatched file compared), modelled on `TestGoldenConformanceMultiFile`.
- [x] **Step 3:** Update the "Phase 2+" exclusion comment at the bottom of the table: remove what is now covered; leave the Phase 3/4 and transform targets with their reasons.
- [x] **Step 4:** Run, triage every failure against the C source first (never patch to the oracle file alone). Expected trouble spots, in likely order: tag ordering of the added `SetUp` tag (test-onlysetup), mark-comment placement/wrapping for fifty/repetition (comments go through the Phase 1 word-wrap path), material match depth off-by-one (test-z/-y), ply-count definition for FEN games (test-b*/p*), CRLF in reltags.txt.
- [x] **Step 5: Commit** — `test: golden conformance rows for pgnx filter`

---

### Task 10: Differential runs + docs + phase close-out

**Files:**
- Modify: `scripts/difftest.sh`, `MIGRATION.md`, `CONFORMANCE.md`, `README.md`, `CLAUDE.md`, this plan's status banner.

- [x] **Step 1: `difftest.sh`** — accept `C_FLAGS` and `PGNX_FLAGS` env vars (verbatim extra args for each binary) and a `PGNX_CMD` (default `convert`) so a filter run is `PGNX_CMD=filter C_FLAGS='-TpFischer' PGNX_FLAGS='--player Fischer' scripts/difftest.sh big.pgn`. Run at least: `--player`, `--date`, `--min-moves`, `--checkmate`, `--repetition --mark-matches X`, `--tag-file reltags.txt`, `--material-file zmatch.txt --mark-matches MATCH` over every `infiles/*.pgn` and over `kyle_b81.pgn` (the Chess.com export in the repo root; keep it untracked or move it under `testdata/local/` in `.gitignore`). Fix or document every divergence.
- [x] **Step 2: `MIGRATION.md`** — move the `-t`/`-T`/`-z`/`-y`/`-b`/`-p`/`-n` and long-flag families from "Not yet ported" into the mapping table with their `filter` equivalents (one row per C flag; include `--repetition5` → `--repetition=5`, `-S` → `--soundex`, `--tagsubstr` → `--tag-substring`, `-n` → `--unmatched`). Keep `-Tf`, `-x`, `-v`, `-e`, dedup flags, `--odds`, and the output transforms under "Not yet ported" with their target phase.
- [x] **Step 3: `CONFORMANCE.md`** — new section "Filtering (Phase 2)" documenting: `=~` uses RE2 not POSIX BRE; malformed criteria files are an error rather than silently truncated; moves bound 0 is a usage error rather than "nothing matches"; pgnx always replays so broken games that fail tag criteria are still reported (exit 2) whereas C never examines them; `--stop-after`/`--select-only` stop before reading the next game (processed count differs); `--keep-broken` evaluates filters against the partial replay. Plus anything Step 1 surfaces. Also verify a Phase 1 gap this phase exposed: `convert` emits a `[Result "1/2"]` or `[Result "?"]` tag unchanged, while the C tool rewrites both in `check_result` (`grammar.c:182`) before output — run `difftest.sh` on a crafted input and either move the normalisation into `output` (with the oracle as justification) or document it.
- [x] **Step 4: `README.md`** — a `filter` section with three examples (player + date, length, repetition with marking, unmatched routing). **`CLAUDE.md`** — add `filter` to the layer list and the subcommand list; mention `scripts/difftest.sh`'s new env vars.
- [x] **Step 5: Full check** — `go build ./... && go test ./... && go vet ./... && gofmt -l .`; flip this plan's banner to **COMPLETE** with the verification date and the commit range.
- [x] **Step 6: Commit** — `docs: filter migration, conformance notes, differential flags; mark Phase 2 complete`

---

## Self-Review (to perform before marking complete)

- **Spec coverage:** every "In" bullet of the spec maps to a task (tags → 2/3, rating/setup → 3/6, bounds → 4, end-state → 4, material → 5, marking → 4/5, routing → 7/8, result consistency → 6, CLI → 8, tests → 9/10).
- **Ordering parity:** `Evaluate`'s chain order and the Selector's held-back rules are each pinned by a unit test *and* by a golden row (test-selectonly, test-n, test-suppressmatched, test-fixresulttags + test-TTr).
- **Side-effect discipline:** the only mutations of `*pgn.Game` are the four documented ones (SetUp tag, Result fix, mark comments, MaterialMatch tag); `grep` for other writes to `g.Tags`/`g.Moves` in `filter/` before closing.
- **Streaming:** no slice of games anywhere in `filter` or `cmd/pgnx/filter.go`.
