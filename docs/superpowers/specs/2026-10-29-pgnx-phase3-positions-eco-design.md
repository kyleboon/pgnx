# pgnx: Go Port of pgn-extract — Phase 3 Design (positions, move sequences, ECO)

**Date:** 2026-10-29
**Status:** Draft (plan: [`docs/superpowers/plans/2026-10-29-pgnx-phase3-positions-eco.md`](../plans/2026-10-29-pgnx-phase3-positions-eco.md))
**Module:** `github.com/kyleboon/pgnx`
**Builds on:** [Phase 1](2026-08-07-pgnx-go-port-design.md) (complete), [Phase 2](2026-10-28-pgnx-phase2-filter-design.md) (complete 2026-10-29)

## Context

Phase 2 delivered `pgnx filter` for every selection criterion that can be
decided from tags, length, end state and material. It deliberately deferred
the criteria that need the *positions* a game passes through and the
*text* of its moves, plus ECO classification, which the C tool performs
during the same replay. Phase 3 adds those:

- **Positional matching** — does the game reach a given position? The C
  tool accepts positions as move sequences (`-x`), FEN strings (`FEN` lines
  in `-t` files), Polyglot hash codes (`-H`), and FEN *patterns* with
  wildcards (`-Tf`, `--fenpattern`, `--fenpatterni`, `FENPattern`/
  `FENPatternI` lines in `-t` files), with `--startply`, `--matchplylimit`,
  `--wtm`/`--btm`, `--markmatches` and `--addlabeltag` modifiers. In C this
  lives in `apply.c` (`play_moves`, `position_matches`, `store_hash_value`),
  `fenmatcher.c` and `moves.c`.
- **Move-sequence matching** — does the game's move text begin with (or
  contain) a given sequence, in any order (`-v`) or exactly (`-v -P`),
  optionally anywhere in the game (`--vanywhere`). `moves.c`
  (`check_textual_variations`, `straight_match`, `permutation_match`).
- **ECO classification** — `-e FILE` adds `ECO`/`Opening`/`Variation`/
  `SubVariation` tags by matching positions against a PGN file of opening
  lines; `-E N` splits the output into files named by ECO prefix. `eco.c`
  plus the classification hooks in `apply.c` and `grammar.c`.

Dedup (`hashing.c`, Phase 4), null moves / `--odds`, and the convert-side
transforms remain out of scope.

## Decisions

| Question | Decision |
|---|---|
| Library shape | Positional and sequence matching are new criteria in the existing `filter` package (`position.go`, `fenpattern.go`, `sequence.go`). ECO classification is a new package `eco` because it is a tag-adding transform, not selection, and `convert` needs it too. `eco` imports `chess`, `pgn`, `replay`; `filter` does not import `eco`; only `cmd/pgnx` composes them. |
| Position identity | The C tool's `weak_hash_value` covers piece placement only (no side to move, castling or en passant; `apply.c:735-750`, `map.c`). pgnx adds `chess.Board.PlacementKey()` with exactly that scope (an XOR over the Polyglot piece table) and exposes it as `replay.Position.Placement`. `-H` compares the full Polyglot hash, which `replay.Position.Zobrist` already is (verified: pgnx computes `db55d4fcaadc775e` for the position the C test uses). Hash *values* are never emitted, so only equality semantics must match. |
| Search depth | `--matchplylimit` and the implicit `-x` depth (`len(line)+8`) stop the positional search; after the limit, an unmatched game is unmatched. pgnx still replays the whole game (as in Phase 2), so a broken move beyond the limit is still reported. Documented divergence, same class as Phase 2's. |
| `--startply` | Threaded through *all* per-ply checks, as in C (`check_for_match` gates repetition, N-move-rule and underpromotion too, `apply.c:1007`). Phase 2 predicates gain the parameter; default 1 keeps their behaviour. |
| Sequence text | C matches against move text that `apply_move` has already rewritten to canonical SAN without check symbols (`rewrite_move`, `apply.c:2091`), except when the output format is Source, where the raw text is used. pgnx always matches against `replay.MoveInfo.SAN` with `+`/`#` stripped. The Source-format quirk is a documented divergence. |
| Permutations default | As in C, `-v` sequences match in any order by default; `--exact-order` (`-P`) requires the listed order. |
| ECO file | No implicit `eco.pgn`-in-cwd or `$ECO_FILE` lookup: `--eco-file FILE` is explicit. The C repo's `eco.pgn` (GPL, 2014 lines) is vendored under `cmd/pgnx/testdata/golden/` for the golden rows and documented as the file to use. |
| ECO split output | `--eco-split N` writes each emitted game to `<ECO[:N]>.pgn` (or `noeco.pgn`) in the current directory, appending, exactly as `-E` does. It conflicts with `--output`. Available on `convert` and `filter`. |
| Pattern trie | `fenmatcher.c` consolidates patterns into a rank trie and the walk order of that trie decides which label wins when several patterns match. pgnx ports the trie rather than a flat list so `--add-label-tag` picks the same label. |
| Tag-file `FEN`/`FENPattern` lines | Phase 2 rejected them; Phase 3 accepts them in `--tag-file` and `--tag`. `FEN "…"` is a *position* (placement key of the FEN); `FENPattern "…" [label]` / `FENPatternI "…" [label]` are patterns, label = rest of line. `-Tf` (`--fen-pattern`) is a pattern, not a position, as in C (`lists.c:320`). |

## Scope

### In

Golden oracle files for everything below are already vendored except
`eco.pgn` (added by this phase).

- **Positions by move sequence** (`-x FILE`, `moves.c:272-360`): each
  non-blank line is a sequence of SAN moves (move numbers optional, stops at
  `*`); the line is replayed from the initial position and the resulting
  placement is a target. Each line raises the default search depth to
  `len+8` plies.
- **Positions by FEN**: `FEN "…"` lines in criteria files and `--fen FEN`
  (`taglines.c:187`, `moves.c:366`). Placement key of the FEN; side to move,
  castling and the clocks in the FEN are ignored.
- **Positions by Polyglot hash** (`-H HEX`, `apply.c:2536`): up to 16 hex
  digits, compared with `replay.Position.Zobrist`.
- **FEN patterns** (`fenmatcher.c`): `-Tf PAT`, `--fenpattern PAT`,
  `--fenpatterni PAT`, `FENPattern`/`FENPatternI` criteria lines, including
  the `:material` constraint suffix, labels, the inverted (`I`) form with the
  `I` label suffix, duplicate-pattern consolidation, and the exact pattern
  grammar (`?`, `!`, `*`, `A`, `a`, `m`, digits, `[…]`, `[^…]`).
- **Modifiers**: `--startply N`, `--matchplylimit N`, `--wtm`, `--btm`,
  `--markmatches TEXT` for position matches (first match only; initial
  position → prefix comment; matches inside variations), `--addlabeltag`
  (`MatchLabel` tag; its output position is already handled by Phase 2's
  tag order).
- **Move sequences** (`-v FILE`, `-P`, `--vanywhere`): `*` any move,
  `!move` disallowed move, `a|b` alternatives, straight and permutation
  matching exactly as `moves.c:442-684`.
- **ECO classification** (`-e FILE`): table built from a PGN file of lines;
  per-ply matching with the C tool's exact/partial rules and 6-ply distance
  limit; replaces `ECO`, `Opening`, `Variation`, `SubVariation`; `-Te`
  criteria evaluated after classification (Phase 2 already runs ECO last).
- **ECO split** (`-E N`, `grammar.c:316`, `eco.c:314`).
- **Seven-tag roster + ECO**: with `--seven-tag-roster` and classification
  on, the four ECO tags are emitted after the roster (`output.c:1531`).

### Out (with owner)

- Duplicate detection (`-D/-U/-d/-c/-Z`, `--fuzzydepth`,
  `--deletesamesetup`) → Phase 4. (`check_duplicate_setup` sits inside the
  Phase 2 chain but is a no-op without `-Z`; still Phase 4.)
- `--odds`, `--allownullmoves` → with null-move support.
- `-A` argument files. `test-FENPattern` feeds its patterns through an
  argsfile whose `:-t` line switches to tag-file mode; the golden row passes
  the same two lines as `--tag` flags instead.
- Convert-side transforms ("Phase 2b"), JSON, `-#`.
- `-L`/`-l` log-file comparison targets (diagnostics are not held to
  parity).

## Architecture

```
github.com/kyleboon/pgnx
├── chess/     + Board.PlacementKey() uint64            (piece placement only; apply.c:735)
├── replay/    + Position.Placement uint64              (set in positionOf)
├── filter/
│     fenpattern.go  PatternSet: Add/parse/invert, rank trie, matcher    (fenmatcher.c)
│     position.go    PositionCriteria: placements, hashes, patterns,
│                    WhoseMove, StartPly, MatchPlyLimit; the per-ply walk
│                    over mainline + variations with marks and label       (apply.c play_moves/position_matches)
│     sequence.go    Sequence: parse, straight/permutation/anywhere match (moves.c)
│     criteria.go    FEN / FENPattern / FENPatternI lines now parsed
│     endstate.go    repetition / N-move / underpromotion take startPly
│     material.go    + (*MaterialPattern).MatchBoard(b) for pattern constraints (end.c:687)
│     filter.go      Options += Positions, Sequences, ExactOrder, SequenceAnywhere, StartPly, AddLabelTag
├── eco/       NEW — Table, Load, Classify, Apply, FilePrefix             (eco.c, apply.c:1196, grammar.c:1505)
├── output/    + Options.RosterExtras (ECO tags after a seven-tag roster)
└── cmd/pgnx/  + filter flags; --eco-file/--eco-split on convert and filter; eco-split file routing
```

Dependency direction unchanged: `filter` and `eco` import `chess`, `pgn`,
`replay`; nothing imports either except `cmd/pgnx`.

## Library API

```go
// Positions.
pc := &filter.PositionCriteria{WhoseMove: filter.EitherToMove, StartPly: 1}
err = pc.AddLine("e4 c5 Nf3 d6 d4 cxd4 Nxd4 Nf6 Nc3 a6 Be3")   // -x line; raises default depth to len+8
err = pc.AddFEN("r3kb1r/pp3ppp/2p1pQ2/3n4/3P4/3B3P/PPP2PP1/R1B2RK1 b kq - 0 13")
err = pc.AddHash("db55d4fcaadc775e")                              // -H
err = pc.AddPattern("*/*/*/*/???Np???/????P???/*/*", true, "Nd4th") // FENPatternI with label
pc.MatchPlyLimit = 150                                            // 0 = C default rule
pcs, err := filter.ReadPositionLines(r)                          // -x file → []string lines (helper)

// Move sequences.
s, err := filter.ParseSequence("e4 c5 Nf3 d6 d4 cxd4 Nxd4 * Nc3 * Be3")
ss, err := filter.ReadSequences(r)                                // -v file

opts := &filter.Options{
    Positions: pc, Sequences: ss, ExactOrder: false, SequenceAnywhere: false,
    StartPly: 1, AddLabelTag: true, MarkMatches: "HERE",
}
matched, err := filter.Evaluate(game, result, opts)

// ECO.
table, err := eco.Load(f)                 // PGN file of lines; broken lines skipped and counted
entry, ok := table.Classify(result)       // last ply with a match wins, as in C
eco.Apply(game, entry)                    // replaces ECO/Opening/Variation/SubVariation
name := eco.FilePrefix(ecoTag, level)     // "A13"→"A1" at level 2; "" or non-alpha → "noeco"
```

## CLI

```
pgnx filter [output flags] [Phase 2 criteria] [positions] [sequences] [file...]
pgnx convert|filter --eco-file FILE [--eco-split N] ...

Positions (a game matches if it reaches ANY listed position/pattern):
    --position "e4 c5 Nf3 ..."   -x line (repeatable)       --position-file FILE   -x FILE
    --fen FEN                    FEN "…" line (repeatable)
    --fen-pattern PAT            -Tf / --fenpattern          --fen-pattern-i PAT    --fenpatterni
    --hash HEX                   -H (repeatable)
    --tag 'FEN "…"' / --tag 'FENPattern "…" label' / --tag-file with such lines
    --start-ply N                --startply (default 1)
    --match-ply-limit N          --matchplylimit
    --white-to-move / --black-to-move     --wtm / --btm
    --add-label-tag              --addlabeltag
    --mark-matches TEXT          (Phase 2 flag; now also marks the first position match)

Move sequences (any order unless --exact-order):
    --sequence "e4 c5 !Nf3"      -v line (repeatable)        --sequence-file FILE   -v FILE
    --exact-order                -P
    --sequence-anywhere          --vanywhere

ECO (convert and filter):
    --eco-file FILE              -e FILE (classification on)
    --eco-split N                -E N (1..10); files <prefix>.pgn / noeco.pgn in cwd, appended;
                                 conflicts with --output
```

Usage errors (exit 1): unparsable `-x` line (C exits 1 too), bad hex for
`--hash`, malformed pattern (C logs and ignores it; pgnx errors — documented),
`--match-ply-limit` below the depth implied by `--position` lines (C exits 1),
`--white-to-move` with `--black-to-move`, `--eco-split` with `--output` or
outside 1..10, `--start-ply` < 1. Exit codes otherwise unchanged.

## Evaluation order (what the port must preserve)

Phase 2's `Evaluate` chain, with the new steps inserted where the C tool
has them (`grammar.c:1214`, `apply.c:955-1330`):

1. Result normalisation; FEN/SetUp consistency; tag criteria (not ECO);
   rated winner; SetUp filter — unchanged.
2. **ECO classification** (CLI, before `Evaluate`): the C tool classifies
   inside `apply_move_list` on both the matched and the `-n` paths, and only
   output reveals it, so classifying every replayed game up front is
   observably identical. Classification replaces the four tags whenever a
   line matched; otherwise existing tags stay.
3. The replay-time block (`play_moves`), per mainline ply `i` with absolute
   ply number `p = p0 + i` where `p0` is the first move's ply number (1 for
   a standard start, `2*(Fullmove-1)+(Black?1:0)+1` from a FEN):
   - Before any move: if positions are being sought, `p0 >= StartPly` and
     the start position matches → matched; mark goes to the **front** of
     the prefix comments.
   - Stop the walk when `p > MatchPlyLimit` and nothing has matched yet
     (the game is then unmatched; later marks are not inserted).
   - `check = p >= StartPly`.
   - Variations on move `i` (unless `--no-variations`): walked first, each
     with its own fresh "matched" state, limit 2000, no N-move marks and no
     repetition counting; a match inside sets the game matched and marks
     the variation move, but never contributes a label.
   - Position after move `i`: if `check` and not yet matched and it matches
     → matched, mark appended to move `i` **before** any repetition or
     N-move mark at the same ply, label recorded (mainline only).
   - Repetition, N-move rule, underpromotion checks only when `check`.
   - Result check / `--fix-result-tags` / `--no-bad-results` at the last
     move — unchanged.
   - Positions sought and nothing matched → unmatched. Then N-move,
     underpromotion, stalemate, insufficient — unchanged.
   - `MatchLabel` tag added when `AddLabelTag` and the label is non-empty.
4. Ply bounds.
5. **Move sequences** (`check_textual_variations`): any sequence matching
   (straight or permutation; from the start, or from any ply with
   `--sequence-anywhere` when the remaining game is at least as long as the
   sequence) → continue, else unmatched.
6. Material, piece count, checkmate, repetition, ECO criteria, commented —
   unchanged.

`position_matches` itself (`apply.c:2585`): placement key in the `-x`/FEN
set, else Polyglot hash in the `-H` set, then the whose-move filter; if that
fails, **still** try the pattern trie (with its material constraint), then
the whose-move filter again. Found-by-key returns an empty label.

ECO matching per ply (`eco.c:276`, `apply.c:1196`): skip when the ply
number exceeds the longest line + 6; among entries with the same placement
key, an exact match (same ply count, side to move and cumulative key)
wins; otherwise an entry with the same side to move whose ply count is at
most 6 less than the game's is a candidate and the **earliest loaded**
candidate wins (the C chain is LIFO and the loop keeps overwriting); the
match found at the latest ply replaces any earlier one.

## Testing

1. **Unit tests** in `filter`: pattern grammar table (every metacharacter,
   digit runs, closures, nested/unbalanced closures rejected, inverse with
   label suffix, duplicate warning), trie walk order with overlapping
   patterns and distinct labels, material-constraint suffix; position walk
   on hand-built games (start-position match → prefix comment, limit
   cut-off, start-ply gating of a repetition mark, match inside a variation
   without label, whose-move fallthrough to patterns); sequence parsing and
   the straight/permutation/anywhere tables lifted from `moves.c` comments
   (`* b6`, `e4 c5 !Nf3`, `cxd|cxd4`, disallowed move found anywhere in the
   permutation window); `-x` line replay errors. In `eco`: table build from
   a small inline file, exact vs partial preference, 6-ply window, earliest-
   loaded tie-break, last-ply-wins, tag replacement incl. removal of a stale
   `Opening`, `FilePrefix` table.
2. **Golden conformance** rows (C command kept as a comment): test-x (+ the
   `-n` variant as a two-output test), test-markmatches, test-v, test-PP,
   test-tFEN, test-FENPattern (argsfile lines passed as `--tag`) and its
   `--fenpattern` CLI variant, test-matchplylimit and test-startply (four
   appended runs each, modelled on the append test), test-e, test-EE (three
   files in a temp cwd). test-Te has no oracle file in the C repo; covered
   by a CLI test instead.
3. **Differential**: `scripts/difftest.sh` with `PGNX_CMD=filter` over the
   golden inputs and the local TWIC sample for `-x`, `-v`, `-Tf`,
   `--fenpatterni`, `-H`, `-e` (and `-e` with `-7`), `-E1`; fix with oracle
   justification or document.

## Non-goals (Phase 3)

- Diagnostics parity (ECO hash-collision warnings, "badly formed" pattern
  logs, "Failed to identify" for `-x` lines).
- Reproducing the Source-format raw-text quirk in sequence matching.
- Performance beyond keeping streaming: the ECO table and the pattern trie
  are built once per run; per-game state is discarded.
