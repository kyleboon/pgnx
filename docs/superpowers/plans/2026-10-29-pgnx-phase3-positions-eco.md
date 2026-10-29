# pgnx Phase 3 Implementation Plan — positions, move sequences, ECO

> **Status: NOT STARTED** (plan written 2026-10-29). Spec: [`docs/superpowers/specs/2026-10-29-pgnx-phase3-positions-eco-design.md`](../specs/2026-10-29-pgnx-phase3-positions-eco-design.md). Tracking: beads epic `pgnx-phase3` (one child task per task below).

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Claim the matching beads task (`bd update <id> --claim`) before starting one and close it when its commit lands.

**Goal:** `pgnx filter` selects games by the positions they reach (move sequences, FENs, Polyglot hashes, FEN patterns with labels and material constraints) and by the text of their opening moves; `pgnx convert|filter --eco-file` classifies games with ECO/Opening/Variation/SubVariation tags and `--eco-split` divides output by ECO prefix; every in-scope golden target byte-matches the C oracle.

**Architecture:** New criteria in `filter` (`fenpattern.go`, `position.go`, `sequence.go`) driven by one per-ply walk that mirrors `play_moves` in `apply.c`; a new `eco` package (table build + classification + tag replacement); one placement-only hash added to `chess`/`replay`; CLI flags on `filter` plus shared ECO flags on `convert` and `filter`.

**Tech Stack:** Go 1.23+, cobra (CLI only), standard library. Reference C code: `~/code/pgn-extract` (flat layout: `apply.c`, `fenmatcher.c`, `moves.c`, `eco.c`, `grammar.c`, `lists.c`, `taglines.c`, `argsfile.c`). Line numbers below refer to that tree as of 2026-10-29.

## Global Constraints

Unchanged from Phases 1–2:

- Module `github.com/kyleboon/pgnx`; GPL-3.0; cobra only under `cmd/`; library packages never `os.Exit`, never panic on user input, never write to stdout/stderr.
- Every task: `go build ./... && go test ./... && go vet ./... && gofmt -l .` clean before committing. Conventional commit messages.
- Oracle precedence when ambiguous: golden `outfiles/` → C source → never guess. Check `CONFORMANCE.md` before "fixing" any output difference.
- **Streaming:** no cross-game state beyond what already exists (selector counters, tag registry) plus the run-wide ECO table and pattern trie, which are read-only after start-up.
- **Dependency direction:** `filter` and `eco` import `chess`, `pgn`, `replay`; only `cmd/pgnx` imports them. `filter` does not import `eco`.
- **Side-effect discipline:** the only new mutations of `*pgn.Game` are position-match mark comments (prefix, mainline, or variation move), the `MatchLabel` tag, and (in `eco.Apply`) the four ECO tags.

## Oracle target → pgnx invocation map

Inputs and oracles are under `cmd/pgnx/testdata/golden/{infiles,outfiles}` already, except `eco.pgn` (Task 9 vendors it from the C repo root).

| C target | C command (on `infiles/`) | pgnx args | oracle |
|---|---|---|---|
| test-x | `-xxvars.txt najdorf.pgn` | `filter --position-file xvars.txt` | test-x-out.pgn |
| test-x (2nd) | `-xxvars.txt -ntest-xn-out.pgn najdorf.pgn` | `filter --position-file xvars.txt -o M --unmatched U` | test-x-out.pgn + test-xn-out.pgn |
| test-markmatches | `--markmatches MATCH -xxvars.txt najdorf.pgn` | `filter --mark-matches MATCH --position-file xvars.txt` | test-markmatches-out.pgn |
| test-v | `-vvvars.txt najdorf.pgn` | `filter --sequence-file vvars.txt` | test-v-out.pgn |
| test-PP | `-P -vPvars.txt test-P.pgn` | `filter --exact-order --sequence-file Pvars.txt` | test-PP-out.pgn |
| test-tFEN | `-ttest-tFEN.txt fischer.pgn` | `filter --tag-file test-tFEN.txt` | test-tFEN-out.pgn |
| test-FENPattern | `-Atest-FENPattern.txt --addlabeltag --markmatches HERE petrosian.pgn` | `filter --tag 'FENPattern "*/*/*/*/???pN???/???P????/*/*"' --tag 'FENPatternI "*/*/*/*/???Np???/????P???/*/*" Nd4th' --add-label-tag --mark-matches HERE` | test-FENPattern-out.pgn |
| test-FENPattern (cli) | `--fenpattern "…" --fenpatterni "…" --markmatches HERE petrosian.pgn` | `filter --fen-pattern '*/*/*/*/???pN???/???P????/*/*' --fen-pattern-i '*/*/*/*/???Np???/????P???/*/*' --mark-matches HERE` | test-FENPattern-cli-out.pgn |
| test-matchplylimit | 4× `-Hdb55d4fcaadc775e --matchplylimit {8,9,10,150} --markmatches match -a…` | 4× `filter --hash db55d4fcaadc775e --match-ply-limit N --mark-matches match --append` | test-matchplylimit-out.pgn |
| test-startply | 4× `--startply {8,9,10,11} -Tf"*/*/4p3/3pP3/2pP4/2P5/*/*" --markmatches match -a…` | 4× `filter --start-ply N --fen-pattern '*/*/4p3/3pP3/2pP4/2P5/*/*' --mark-matches match --append` | test-startply-out.pgn |
| test-e | `-e../eco.pgn test-e.pgn` | `convert --eco-file eco.pgn` | test-e-out.pgn |
| test-EE | `-e../eco.pgn -E1 test-ucE.pgn` (cwd output) | `convert --eco-file eco.pgn --eco-split 1` in a temp cwd | A.pgn, B.pgn, E.pgn |
| test-Te | `-e../eco.pgn -TeB14 fischer.pgn` | `filter --eco-file eco.pgn --eco B14` | *no oracle file in the C repo* → CLI test asserts ≥1 game and every emitted ECO tag starts with B14 |

Excluded: dedup targets (Phase 4), test-odds/test-allownullmoves (null moves), test-AA (argsfile), output-transform targets, log-only targets.

---

### Task 1: placement key in `chess` and `replay`

**Files:**
- Modify: `chess/zobrist.go`, `replay/replay.go`
- Test: `chess/zobrist_test.go`, `replay/replay_test.go`

**Interfaces:**
- `func (b *Board) PlacementKey() uint64` — XOR of `polyglotRandom[64*kind+8*rank+file]` over occupied squares only; no castling, en passant or side-to-move terms. This is the pgnx stand-in for `weak_hash_value` (`apply.c:735-750`; updated incrementally in `map.c` by XOR of the same kind of per-square term), whose only observable property is equality.
- `replay.Position` gains `Placement uint64`, set in `positionOf`.

- [x] **Step 1: Failing tests.** `PlacementKey` equal for two boards differing only in side to move / castling rights / ep square; different after a single move; `Zobrist` still differs in those cases. `replay`: `Positions[i].Placement == FromFEN(Positions[i].FEN).PlacementKey()` for every ply of fools-mate.
- [x] **Step 2: Implement.**
- [ ] **Step 3: Test, vet, commit** — `feat(chess,replay): piece-placement key for positional matching`

---

### Task 2: `filter` — FEN patterns (`fenmatcher.c`)

**Files:**
- Create: `filter/fenpattern.go`
- Modify: `filter/material.go` (add `MatchBoard`)
- Test: `filter/fenpattern_test.go`, `filter/material_test.go`

**Interfaces:**
- `type PatternSet struct { root *patternNode }` (rank trie: `rank string; label string; constraint *MaterialPattern; alt, next *patternNode`).
- `func (s *PatternSet) Add(pattern string, inverse bool, label string) error` — port of `add_fen_pattern` (`fenmatcher.c:93-217`): scan to `' '`, `':'` or end; exactly 7 `/` dividers, every rank non-empty; `[` not nested, `]` must close, `^` only inside `[…]`; the text after `':'` is a material constraint parsed with `ParseMaterial(rest, EitherColour-if-inverse-else-WhiteFirst)` (`process_material_description(p, add_reverse, TRUE)`; note `both_colours = add_reverse`). Insert via the trie rules of `pattern_tree_insert`/`insert_pattern` (`fenmatcher.c:276-340`): walk while ranks are `==`; an identical pattern is a duplicate (ignored; return a sentinel `ErrDuplicatePattern` the CLI may log); otherwise append as `alt` at the first differing rank, recursing down existing `alt` chains. When `inverse`, also add `reverseFENPattern(p)` (ranks reversed, letter case swapped, `fenmatcher.c:220`) with label `label+"I"` (or `""` when the label is empty).
- `func (s *PatternSet) Match(b *chess.Board) (label string, ok bool)` — `pattern_match_board`/`pattern_match_rank` (`fenmatcher.c:344-400`): rank 8 first; ranks rendered lazily as 8-char strings with `_` for empty (`convert_rank_to_text`); at the last rank, a `constraint` must also satisfy `MatchBoard`; on failure try `alt`.
- Rank matcher — literal port of `matchhere`/`matchstar`/`matchone`/`matchccl`/`matchnccl` (`fenmatcher.c:404-582`): `?` any square, `!` non-empty, `A`/`a` any white/black piece, `m` not a pawn, `*` zero or more squares (leftmost-longest, tried longest first), digits = exactly that many empty squares, `[…]` any listed (metachars allowed inside), `[^…]` none listed; the pattern must consume the whole rank.
- `func (p *MaterialPattern) MatchBoard(b *chess.Board) bool` — `constraint_material_match` (`end.c:687`): depth 0, white set vs board, and black set too when `bothColours`.
- `func (s *PatternSet) Empty() bool`.

- [x] **Step 1: Failing tests.** Grammar table: valid 8-rank pattern; 7 ranks → error; empty rank (`//`) → error; `[[` → error; `]` alone → error; `^` outside closure → error; trailing ` w KQkq` ignored; `:kq k` constraint parsed. Matching table on `FromFEN` boards: `*/*/*/*/???pN???/???P????/*/*` matches the position after `1.e4 e5 2.Nf3 d6 3.d4 Ne4`-style knight-on-e4 setups and not without the supporting pawn; digits exact (`8` vs a rank with 7 empties fails); `!`, `A`, `a`, `m`; `[NB]` and `[^P_]`; `*` at both ends and in the middle; a constraint that fails rejects a rank-matching board. Inverse: label `X` → `XI`; the reversed pattern of `*/*/*/*/???Np???/????P???/*/*` matches a Black knight on d5 supported by e6. Trie order: patterns P1, P2 (differ at rank 1), P3 (shares rank 1 with P1, differs at rank 2) — a board matching P2 and P3 yields P3's label (C walk order P1, P3, P2). Duplicate returns `ErrDuplicatePattern` and does not change results.
- [x] **Step 2: Implement.**
- [ ] **Step 3: Test, vet, commit** — `feat(filter): FEN pattern matching with labels and material constraints`

---

### Task 3: `filter` — positional criteria and the per-ply walk (`apply.c` play_moves)

**Files:**
- Create: `filter/position.go`
- Modify: `filter/endstate.go` (startPly parameter), `filter/filter.go` (new Options fields and the walk's place in `Evaluate`)
- Test: `filter/position_test.go`, `filter/endstate_test.go`, `filter/filter_test.go`

**Interfaces:**
- `type WhoseMove int` — `EitherToMove`, `WhiteToMove`, `BlackToMove` (`defs.h:49`).
- `type PositionCriteria struct { WhoseMove WhoseMove; MatchPlyLimit int /* 0 = default rule */; Patterns PatternSet; placements, hashes map[uint64]struct{}; lineDepth int }`
  - `AddLine(line string) error` — `compose_positional_variation` (`moves.c:272`): split on whitespace, strip leading digits and dots from each token, stop at a token starting with `*`; replay the SAN list from the standard start via a synthetic `pgn.Game` + `replay.Game`; error on an illegal/unknown move (C exits 1); record the final `Placement`; `lineDepth = max(lineDepth, len+8)`.
  - `AddFEN(fen string) error` — `chess.FromFEN(fen).PlacementKey()`.
  - `AddHash(hex string) error` — `save_polyglot_hashcode` (`apply.c:2536`): 1–16 hex digits.
  - `AddPattern(p string, inverse bool, label string) error` — delegates to `Patterns.Add`.
  - `Empty() bool`; `effectiveLimit() int` = `MatchPlyLimit` if > 0, else `lineDepth` if > 0, else 2000 (`DEFAULT_POSITIONAL_DEPTH`, `apply.c:52`). The CLI enforces `MatchPlyLimit >= lineDepth` (C: `argsfile.c:1372`).
  - `matches(pos replay.Position, b func() *chess.Board) (label string, ok bool)` — `position_matches` (`apply.c:2585`): placement in set, else Zobrist in set → found with `""`; apply whose-move; if not found, `Patterns.Match(board)` then whose-move again. The board is built lazily from the FEN only when patterns exist.
- `func matchPositions(g *pgn.Game, r *replay.Result, pc *PositionCriteria, startPly int, mark string, noVariations bool) (matched bool, label string, stoppedAt int)` — the walk in the spec's "Evaluation order" step 3. `stoppedAt` is the mainline ply index at which the depth limit ended the walk (`len(g.Moves)` if it did not), so `Evaluate` can skip the repetition/N-move/underpromotion marks beyond it. Variation recursion: for `g.Moves[i].Variations[j]` with `r.Moves[i].Variations[j]`, call the same walk with a fresh matched state, limit 2000, `mainline=false`; OR the result; discard the label. Mark text: `mark`, or the position's FEN when `mark == "FEN"` (`create_match_comment`, `apply.c:2999`) — reuse Phase 2's `markComment`.
- `Options` gains: `Positions *PositionCriteria`, `StartPly int` (0 ⇒ 1), `AddLabelTag bool`. `matchRepetition`, `matchNMoveRule`, `matchUnderpromotion` gain `startPly int` (and the mainline cut-off index): a check at ply `i` (0-based) runs only when `p0+i >= startPly` where `p0 = 2*(Positions[0].FullmoveNumber-1) + (Black?1:0) + 1`.
- `Evaluate` changes: after `ensureValidResultTag`, run `matchPositions` first (so its marks precede repetition/N-move marks at the same ply — insert position marks during the walk; repetition/N-move marks are added afterwards by their own passes, which is the same final order because each move's `Comments` slice is appended in walk order: position first, then repetition, then N-move). If `Positions` is non-empty and `matched` is false → return false **after** the result check? No: C returns from `play_moves` with `game_matches=false` after the loop, but the end-of-game result check (`--no-bad-results` ⇒ reject) only runs when the walk reached the last move. Port: if the walk stopped early, skip `checkResult`; otherwise run it; then if positions sought and unmatched → false. Then `MatchLabel` tag when `AddLabelTag && label != ""` (`setTag`).

- [x] **Step 1: Failing tests.** `AddLine("e4 c5 Nf3 d6 d4 cxd4 Nxd4 Nf6 Nc3 a6 Be3")` gives depth 19 and the game `1.e4 c5 2.Nf3 d6 3.d4 cxd4 4.Nxd4 Nf6 5.Nc3 a6 6.Be3` matches at ply 11 with `{ MATCH }` after `Be3`; a transposition reaching the same position at ply 25 with no limit override does *not* match (depth 19) but does with `MatchPlyLimit: 30`; `AddLine("e4 e5 Qh9")` errors. `AddFEN` of a position with Black to move matches the same placement reached with White to move (and fails under `WhoseMove: BlackToMove`, but still matches via a pattern that happens to fit — whose-move fallthrough). `AddHash("db55d4fcaadc775e")` matches after `5.Nc3` only with limit ≥ 9 (limits 8/9 table). Start-position match (`AddFEN(startpos)`) puts the mark at the front of `PrefixComments`. `StartPly: 8` suppresses a repetition mark that Phase 2 placed at ply 6 and keeps one at ply 10. A match inside a RAV marks the RAV move, matches the game, and yields no label. `AddLabelTag` adds `MatchLabel` only for a non-empty pattern label. The walk stopping at the limit skips `checkResult` (a game with a bad Result and no match is simply unmatched, not `ErrBadResult`).
- [x] **Step 2: Implement.**
- [ ] **Step 3: Test, vet, commit** — `feat(filter): positional matching by move line, FEN, hash and pattern`

---

### Task 4: `filter` — move sequences (`moves.c`)

**Files:**
- Create: `filter/sequence.go`
- Modify: `filter/filter.go` (Options + chain position)
- Test: `filter/sequence_test.go`, `filter/filter_test.go`

**Interfaces:**
- `type Sequence struct { moves []seqMove /* text, any, disallowed */; whiteAny, blackAny, whiteDis, blackDis int }`
- `func ParseSequence(line string) (*Sequence, error)` — `compose_variation` (`moves.c:150`): whitespace split; strip leading digits/dots; skip number-only tokens; `*`-prefixed = any (count per colour by index parity; C only warns if more text follows); `!`-prefixed = disallowed; empty line → nil.
- `func ReadSequences(r io.Reader) ([]*Sequence, error)` — one per non-blank line (`add_textual_variations_from_file`). Note C prepends, so file order is reversed; only "any matches" is observable, so keep file order.
- `func textualMatch(variationMove, actual string) bool` — `moves.c:397`: `actual` found in `variationMove` bounded by non-move characters (`move_char` = alnum or `-`).
- `func (s *Sequence) straight(moves []string) bool`, `permutation(moves []string) bool` — literal ports of `moves.c:442-684` including the two-stage disallowed scan, the per-colour `any` slack, and the `matched == len` completion check.
- `func matchSequences(r *replay.Result, seqs []*Sequence, exact, anywhere bool) bool` — `check_textual_variations` (`moves.c:775`): game move list = `MoveInfo.SAN` with trailing `+`/`#` removed; `anywhere` slides the start while any sequence still fits.
- `Options` gains `Sequences []*Sequence`, `ExactOrder bool`, `SequenceAnywhere bool`. In `Evaluate`, between bounds and material.

- [ ] **Step 1: Failing tests.** Parse table (`1. e4 c5 2. Nf3` → 3 moves; `* b6` → whiteAny 1; `e4 c5 !Nf3` → blackDis… check parity: index 2 is White → `whiteDis`; `cxd|cxd4`). `textualMatch("Nc6|Nf3|f3", "f3")` true, `("…", "c6")` false. Straight: Pvars line vs `1.c4 Nf6 2.Nc3 e6 3.d4 Bb4` true, vs `1.d4 Nf6 2.c4 e6 3.Nc3 Bb4` false; permutation: both true. Disallowed: `e4 c5 !Nf3` rejects a game with 2.Nf3 anywhere in the 3-ply window under permutation, and only at index 2 under straight. `*` slack consumed per colour. Game shorter than sequence → false. Anywhere: sequence `Nc3 a6 Be3` matches `… 5.Nc3 a6 6.Be3` only with `anywhere`. Check suffix stripped: `Bxc3+` in the game matches `Bxc3` in the sequence.
- [ ] **Step 2: Implement.**
- [ ] **Step 3: Test, vet, commit** — `feat(filter): move-sequence matching with permutations and anywhere`

---

### Task 5: `filter` — FEN/FENPattern criteria lines and the full `Evaluate` order

**Files:**
- Modify: `filter/criteria.go`, `filter/tags.go`, `filter/filter.go`, `filter/doc.go`
- Test: `filter/criteria_test.go`, `filter/filter_test.go`

**Interfaces:**
- `Criterion` gains `Label string`. `ParseCriterion` accepts `FEN`, `FENPattern`, `FENPatternI` (`taglines.c:183-211`): value quoted as usual; for the two pattern names, the rest of the line after the closing quote (whitespace-trimmed) is the label (empty allowed). `TagMatcher.Add` returns an error for these three names (they are positional, not tag criteria); a helper `func (c Criterion) IsPositional() bool` lets the CLI route them to `PositionCriteria` (`FEN` → `AddFEN`; `FENPattern` → `AddPattern(v, false, label)`; `FENPatternI` → `AddPattern(v, true, label)`).
- `Evaluate` final order (spec "Evaluation order"); update the doc comment and `doc.go`.

- [ ] **Step 1: Failing tests.** `FEN "…"` parses with `Label == ""`; `FENPatternI "…" Nd4th` parses with label `Nd4th`; `FENPattern "…"   ` (trailing spaces) label empty; `TagMatcher.Add` rejects them. An ordering test: a game that matches a position but fails bounds is unmatched and keeps its position mark; a game failing a sequence is not tested for material (no material mark inserted); ECO criteria still last.
- [ ] **Step 2: Implement.**
- [ ] **Step 3: Test, vet, commit** — `feat(filter): positional criteria lines and Phase 3 evaluation order`

---

### Task 6: `eco` package

**Files:**
- Create: `eco/eco.go`, `eco/doc.go`
- Test: `eco/eco_test.go`

**Interfaces:**
- `type Entry struct { ECO, Opening, Variation, SubVariation string; present [4]bool }` — a tag is written only if the line had it (`apply.c:1301-1314`).
- `type Table struct { byKey map[uint64][]entry /* file order */; maxHalfMoves int; Skipped int }`
- `func Load(r io.Reader) (*Table, error)` — `deal_with_ECO_line` + `save_eco_details` (`grammar.c:1505`, `eco.c:155`): scan with `pgn.Scanner`; replay each game (FEN tag honoured); `entry = {Placement of final position, cumulative = wrapping sum of Placement after each mainline move, halfMoves = ply number of the final position (2*(Fullmove-1)+(Black?1:0)), toMove, tags}`; an entry equal in all four of (key, halfMoves, toMove, cumulative) to an existing one is skipped (C logs a collision); `maxHalfMoves = max(halfMoves)+6` (initial 6, `ECO_HALF_MOVE_LIMIT`); a line that fails replay is skipped and counted in `Skipped`; a scanner error is returned.
- `func (t *Table) Classify(r *replay.Result) (Entry, bool)` — `eco_matches` (`eco.c:276`) at each mainline ply ≥ 1 (`apply.c:1196`): `half = ply number of Positions[i]`, `cum += Positions[i].Placement`; skip when `half > maxHalfMoves`; candidates = `byKey[Placement]`; exact = same `halfMoves`, `toMove`, `cum` → take it; else first candidate in file order with same `toMove` and `0 <= half-halfMoves <= 6` (C: LIFO chain, last overwrite wins ⇒ earliest loaded). Any match at a later ply replaces an earlier one. Mainline only; the start position is never tested.
- `func Apply(g *pgn.Game, e Entry)` — remove every `ECO`, `Opening`, `Variation`, `SubVariation` tag, then append those present in `e`, in that order.
- `func FilePrefix(ecoTag string, level int) string` — `open_eco_output_file` (`eco.c:314`): empty or non-letter first char → `"noeco"`; else first `level` bytes.

- [ ] **Step 1: Failing tests.** Inline ECO file with: `A00 1.b4`, `A00/Tuebingen 1.b4 Nh6`, a line with Opening but no Variation, a transposition pair (same final position via two orders) and a 2-ply-longer sibling. Classify: `1.b4 Nh6` → Tuebingen (exact beats the 1-ply `A00`); `1.b4 Nf6` → `A00` partial; a game reaching the transposition position by a third order → earliest-loaded; a game 7 plies past the longest line keeps the last match found inside the window; a game with 0 moves → no match. `Apply` removes a stale `Opening` when the entry has none and keeps unrelated tags. `FilePrefix("A13",1)=="A"`, `("A13",2)=="A1"`, `("A13",5)=="A13"`, `("",1)=="noeco"`, `("?",1)=="noeco"`. `Load` on `testdata/golden/eco.pgn` (vendored in Task 9; until then use a temp copy from `~/code/pgn-extract/eco.pgn`) yields 2014 entries minus duplicates, `Skipped == 0`.
- [ ] **Step 2: Implement.**
- [ ] **Step 3: Test, vet, commit** — `feat(eco): ECO classification table and tag replacement`

---

### Task 7: `output` — ECO tags after a seven-tag roster

**Files:**
- Modify: `output/output.go`
- Test: `output/output_test.go`

**Interfaces:**
- `Options.RosterExtras []string` — when `SevenTagRoster` is set, tags named here are emitted (if present on the game) after the seven, in the listed order (`output.c:1531-1539`: ECO, Opening, Variation, SubVariation, then the FEN/SetUp pair the Writer already handles). Ignored when `SevenTagRoster` is false.

- [ ] **Step 1: Failing test.** A game with ECO/Opening/Variation and an `Annotator` tag under `SevenTagRoster` + `RosterExtras{ECO,Opening,Variation,SubVariation}` emits the seven, then the three present ECO tags, not `Annotator`; without `RosterExtras` only the seven.
- [ ] **Step 2: Implement; commit** — `feat(output): roster extras for ECO tags under --seven-tag-roster`

---

### Task 8: CLI — filter flags, shared ECO flags, eco-split routing

**Files:**
- Modify: `cmd/pgnx/filter.go`, `cmd/pgnx/convert.go`, `cmd/pgnx/games.go` (if a per-game writer selector is needed), `cmd/pgnx/root.go`
- Create: `cmd/pgnx/eco.go` (shared ECO flag registration/parsing + split-file writer map)
- Test: `cmd/pgnx/filter_test.go`, `cmd/pgnx/convert_test.go`, `cmd/pgnx/eco_test.go`

**Interfaces:**
- `filter` flags exactly as the spec's CLI section: `--position` (StringArray), `--position-file` (StringArray), `--fen`, `--fen-pattern`, `--fen-pattern-i`, `--hash`, `--start-ply` (Int, default 1), `--match-ply-limit`, `--white-to-move`, `--black-to-move`, `--add-label-tag`, `--sequence`, `--sequence-file`, `--exact-order`, `--sequence-anywhere`. `filterOptions` routes positional criteria lines from `--tag`/`--tag-file` (Task 5) and validates: both to-move flags ⇒ error; `--match-ply-limit` < implied depth ⇒ error quoting both numbers; `--start-ply` < 1 ⇒ error; `--exact-order`/`--sequence-anywhere` without sequences ⇒ accepted no-ops (as C).
- `--position-file`/`--sequence-file`: `%`-comment and blank-line tolerant via `ReadPositionLines`/`ReadSequences`; a replay failure in a position line is a usage error naming the file and line.
- `addECOFlags(cmd)` registered by both `convert` and `filter`: `--eco-file FILE`, `--eco-split N`. `ecoOptions(cmd)` loads the table once (usage error if unreadable; log `Skipped` count unless `--quiet`), validates `--eco-split` (1..10, not with `--output`, requires nothing else — C allows `-E` without `-e`, splitting on existing ECO tags).
- Per-game: after replay (and for broken games under `--keep-broken` with a partial result), `eco.Apply(g, entry)` when classification is on and `Classify` matched; then `filter.Evaluate` (filter) or straight to output (convert). With `--eco-split`, the destination writer is chosen per game by `eco.FilePrefix(ecoTag, N)+".pgn"`, opened in append mode on first use and kept open for the run (one `output.Writer` per file, all sharing the run's `TagRegistry` and options; close all at exit). `--seven-tag-roster` with `--eco-file` sets `RosterExtras`.
- `MIGRATION.md`-visible naming is final here; keep flag help strings citing the C flag.

- [ ] **Step 1: Failing CLI tests** (`runCLI`): `--position "e4 c5"` selects a Sicilian and routes the other to `--unmatched`; `--fen-pattern` with `--mark-matches X` places `{ X }`; `--hash` with a bad value ⇒ exit 1; `--match-ply-limit 5` with a 6-ply `--position` ⇒ exit 1; `--white-to-move --black-to-move` ⇒ exit 1; `--sequence "c4 Nf6 Nc3"` vs `--exact-order`; `--tag-file` containing a `FENPattern` line with a label and `--add-label-tag` adds `MatchLabel`; `convert --eco-file` on a Reti game adds `[ECO "A04"]` (file: temp copy of eco.pgn until Task 9); `--eco-split 1` in a temp cwd creates `A.pgn` and `noeco.pgn` and refuses `--output`; `filter --eco-file … --eco B14` emits only B14 games (the test-Te stand-in).
- [ ] **Step 2: Implement.** Keep `convert` byte-identical when no ECO flag is given (existing conformance rows guard it).
- [ ] **Step 3: Test, vet, commit** — `feat: positional, sequence and ECO flags for pgnx filter and convert`

---

### Task 9: Golden conformance rows

**Files:**
- Add: `cmd/pgnx/testdata/golden/eco.pgn` (copied verbatim from `~/code/pgn-extract/eco.pgn`; note its origin and licence in the existing testdata README/comment)
- Modify: `cmd/pgnx/conformance_test.go`

- [ ] **Step 1:** Add a row per line of the map above with the C command as a comment. Rows needing `--eco-file` pass `filepath.Join("testdata","golden","eco.pgn")`.
- [ ] **Step 2:** `TestGoldenConformanceUnmatched` gains the test-x two-output case (or a sibling table). Add `TestGoldenConformanceAppendRuns` for test-matchplylimit and test-startply (four `--append` runs into one file, compared once), modelled on `TestGoldenConformanceAppend`. Add `TestGoldenConformanceECOSplit` running `convert --eco-file … --eco-split 1` in a temp cwd and comparing `A.pgn`, `B.pgn`, `E.pgn` (assert no other `*.pgn` was created).
- [ ] **Step 3:** Update the "Phase 3+" exclusion comment: remove what is now covered, leave Phase 4/null-move/transform/argsfile targets with reasons; note test-Te's missing oracle.
- [ ] **Step 4:** Run and triage every failure against the C source first. Likely trouble spots, in order: mark-comment placement when a variation precedes the mainline move (comment goes on the RAV move, not the mainline), prefix-comment ordering for a start-position match, `{ match }` wrapping, ECO tag order under the default roster (`wellKnownTagOrder` already has ECO/Opening/Variation/SubVariation after `Result`; verify against test-e-out), the `--append` runs sharing one `TagRegistry` per process in the test (each C run is a separate process — run each pgnx invocation as a separate `runCLI` call), `noeco.pgn` creation when no game lacks an ECO (C creates files only on write).
- [ ] **Step 5: Commit** — `test: golden conformance rows for positional, sequence and ECO matching`

---

### Task 10: Differential runs, docs, close-out

**Files:**
- Modify: `MIGRATION.md`, `CONFORMANCE.md`, `README.md`, `CLAUDE.md`, this plan's status banner, the spec's status line.

- [ ] **Step 1: Differential.** With `scripts/difftest.sh` (`PGNX_CMD=filter|convert`, `C_FLAGS`, `PGNX_FLAGS`) over every `infiles/*.pgn` and one TWIC issue (`twic/` local corpus; see the `twic-differential-corpus` memory): `-x xvars.txt`, `-v vvars.txt`, `-P -v Pvars.txt`, `-Tf …`, `--fenpatterni …`, `-H …`, `-e eco.pgn`, `-e eco.pgn -7`, `-e eco.pgn -TeB1`, `--startply 20 -Tf …`, `--matchplylimit 20 -x …`. Fix with oracle justification or document.
- [ ] **Step 2: `MIGRATION.md`.** Move `-x`, `-v`, `-P`, `--vanywhere`, `-H`, `-Tf`, `--fenpattern(i)`, `FEN`/`FENPattern` lines, `--startply`, `--matchplylimit`, `--wtm`/`--btm`, `--addlabeltag`, `-e`, `-E` into the mapping table; update the "Not yet ported" list (Phase 4 dedup, null moves, transforms, argsfile) and the Phase-2 note that FEN lines are rejected.
- [ ] **Step 3: `CONFORMANCE.md`.** New section "Positional matching and ECO (Phase 3)": pgnx replays past the search depth (broken games beyond `--match-ply-limit` are reported); malformed patterns and unreplayable `-x` lines are usage errors; sequence matching uses canonical SAN even in Source format; no implicit `eco.pgn`/`$ECO_FILE`; ECO collision/skip diagnostics differ; whatever Step 1 surfaces.
- [ ] **Step 4: `README.md`** — positional/sequence/ECO examples. **`CLAUDE.md`** — add `eco` to the layer list; mention `--eco-file`/`--eco-split`; update the phase status line.
- [ ] **Step 5: Full check** — `go build ./... && go test ./... && go vet ./... && gofmt -l .`; flip this plan's banner to **COMPLETE** with the verification date and commit range; set the spec's status to Implemented.
- [ ] **Step 6: Commit** — `docs: positional/ECO migration and conformance notes; mark Phase 3 complete`

---

## Self-Review (before marking complete)

- **Spec coverage:** positions by line/FEN/hash → 3; patterns → 2; modifiers (`--startply`, `--matchplylimit`, whose-move, marks, labels) → 3/5/8; sequences → 4; ECO classification → 6; ECO split → 8; roster + ECO → 7; criteria lines → 5; goldens → 9.
- **Ordering parity:** position marks before repetition marks at the same ply, variation-before-mainline marking, prefix-comment prepend, sequence check between bounds and material, ECO criteria last — each pinned by a unit test and (where an oracle exists) a golden row.
- **Side-effect discipline:** grep `filter/` and `eco/` for writes to `g.Tags`/`g.Moves`/`g.PrefixComments`; only the documented ones.
- **Streaming:** no slice of games anywhere; ECO table and pattern trie built once; `--eco-split` writer map bounded by the number of distinct prefixes.
