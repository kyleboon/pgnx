# Conformance notes

pgnx is tested against pgn-extract's C implementation as a behavioral
oracle, via a golden-corpus harness (`cmd/pgnx/conformance_test.go`,
vendoring the C repo's `test/infiles`/`test/outfiles`) and a differential
script (`scripts/difftest.sh`) that runs both binaries over the same input
and diffs their output. This document lists known, intentional differences
— and a couple of open concerns surfaced by differential testing — so a
divergence can be triaged quickly instead of re-investigated from scratch.

Game/movetext output (the primary product) is held to byte-for-byte
parity wherever both tools implement the same feature. Diagnostics
(error/log wording and destination) and exit codes are explicitly *not*
held to parity; see below.

## Process-level differences

- **Exit codes.** pgnx uses 0 (success), 1 (usage/flag error), 2 (one or
  more games failed to parse/validate/convert). pgn-extract mostly exits 0
  even when individual games are rejected (it only signals failure via its
  diagnostic output, not the process exit status). Scripts that check
  pgnx's exit code to detect bad input should expect this; scripts ported
  from wrapping the C binary should not assume 0 means "no problems."
- **Diagnostics wording and destination.** Per-game error messages and the
  run summary are not required to match the C tool's text. pgnx's
  `--log FILE` is the equivalent of `-l`/`-L`, but message wording,
  formatting, and exact line numbers/columns differ. Only the emitted PGN
  (or other format) output is held to byte parity.

## Feature-scope differences

- **Null moves (`--`) are rejected.** pgn-extract accepts `--` as a null
  move when `--allownullmoves` is given (and always within variations).
  pgnx's replay does not implement null-move decoding in Phase 1: any game
  containing `--` in the main line fails replay and is dropped by
  `convert`/`validate` regardless of flags. Confirmed against
  `testdata/golden/infiles/test-allownullmoves.pgn` and against `odds.pgn`
  (games 29 and 31, which embed `{1.e4 --}`/`{1.d4 --}` null moves in a
  comment before the real second move) — pgn-extract accepts these lines,
  pgnx does not.
- **`-W` German/language-specific piece-letter output is unsupported.**
  pgn-extract's `-Wsan[PNBRQK]` form (e.g. `-WsanBSLTDK`, exercised by the
  C test suite's `test-WWdeutsch` target) lets the output use
  language-specific piece letters. pgnx's `-f`/`--format` has no CLI
  surface for alternative piece-letter alphabets.
- **CM (ChessMaster) format is unsupported.** pgn-extract's `-Wcm` output
  format has no pgnx equivalent.
- **JSON output and argument files are unsupported.** `--json` and
  `-A`/argsfile support are deferred to a later phase (see MIGRATION.md).
- **`--keep-broken` always emits in Source format.** pgn-extract's
  `--keepbroken` re-emits a broken game in whatever `-W` format was
  requested. pgnx's `convert --keep-broken` always emits broken games in
  Source format (SAN exactly as parsed) regardless of `-f`, since the
  replay detail (`replay.Result`) required by non-Source formats is
  unavailable for a game that failed replay.
- **`validate`/`convert` I/O error lines go to stderr/`--log`, not
  stdout.** Unreadable input files are reported on the log writer
  (stderr by default, or `--log FILE`); they are not folded into
  `validate`'s findings stream on stdout, so `--summary` does not
  suppress them.

## Parser data-model differences

- **NAG/comment interleaving order is not preserved.** `pgn.Move` stores
  `NAGs` and `Comments` as two separate slices with no shared ordering
  information, so pgnx's SAN/Source output always renders all of a move's
  NAGs before all of its comments. pgn-extract preserves the original
  interleaving (e.g. `$1 { c } $2` can round-trip as written). Confirmed
  divergent on `testdata/golden/infiles/test-nagcomments.pgn`: pgn-extract
  emits `$1 { good or bad? } $2 { some think this is good } ...`, pgnx
  emits `$1 $2 { good or bad? } { some think this is good } ...`. This is
  a parser data-model change (a combined ordered token list per move), not
  a minimal output fix, so it is deliberately deferred rather than patched
  around. The C repo's `test-nagcomments` Makefile target is excluded from
  the golden harness on this basis.
- **Comments attached across a variation's closing paren are not
  preserved in position.** Similarly, a comment appearing after a
  variation's `)` but conceptually trailing the move before the `(`
  (`... 2. d4 (2. Nf3){ c4 } ) { c5 } 2. Nf3`) is reordered relative to
  the C oracle's output, for the same reason as the NAG/comment case above
  — comments are stored per-move/per-variation without a token-level
  ordering that spans a variation boundary. Confirmed divergent on
  `testdata/golden/infiles/test-splitvariants.pgn`.

## Filtering (Phase 2: `pgnx filter`)

`pgnx filter` ports pgn-extract's game-selection criteria (`lists.c`,
`taglines.c`, `end.c`, the bounds/end-state checks in `moves.c` and
`apply.c`, repetition counting in `hashing.c`) and evaluates them in the
same order as `grammar.c`'s `deal_with_game` chain, so which games reach
`--output` versus `--unmatched`, where `--mark-matches` comments land, and
what `--result` criteria see relative to `--fix-result-tags` all match the
oracle. Every in-scope target of the C test suite is a golden row
(`cmd/pgnx/conformance_test.go`), and a differential sweep of 33 flag
combinations over 15 inputs (every relevant golden input plus a 31-game
Chess.com export) produced byte-identical output. Known, intentional
differences:

- **`=~` uses Go RE2 syntax, not POSIX basic regular expressions.**
  pgn-extract compiles `=~` criteria with `regcomp(..., 0)` (POSIX BRE,
  where `+`, `?`, `|`, `(`, `)` are literals unless backslash-escaped).
  pgnx uses Go's `regexp` (RE2, Perl-like syntax). Patterns using only
  `.`, `*`, `^`, `$` and character classes behave the same; patterns
  relying on BRE-specific escaping differ. The golden corpus has no `=~`
  case.
- **Malformed criteria files are an error, not silently truncated.** The
  C tool stops reading a `-t` file at the first line that does not start
  with a tag name (`taglines.c` `process_tag_line` returns FALSE, and
  reading ends), and merely logs a line with a missing quoted value.
  pgnx's `--tag-file`/`--tag` reject any line that does not parse
  (`filter.ReadCriteria` names the line number) with exit status 1.
  `FEN`, `FENPattern` and `FENPatternI` lines are rejected with a
  "positional criteria are not supported yet" error (Phase 3).
- **A length bound of 0 is a usage error.** `-bl0`/`-b0` in C compute
  `2*(0-1)+1` in unsigned arithmetic and silently match nothing; `-bu`
  below `-bl` is logged and ignored. pgnx rejects `--moves`/`--min-moves`/
  etc. of 0, and a lower bound above the upper bound, with exit status 1.
- **pgnx always replays, so broken games that fail tag criteria are still
  reported.** The C tool skips `apply_move_list` for a game that fails the
  tag checks (unless `-n` is given), so an illegal move in such a game is
  never noticed. pgnx replays every game before evaluating criteria; a
  broken game is logged and makes the exit status 2 whether or not it
  would have matched. Output is unaffected (broken games are emitted
  nowhere unless `--keep-broken`).
- **`--stop-after`, `--select-only` and `--game-limit` stop before reading
  the next game.** The C tool parses one more game after the stop
  condition is met and discards it (`parse_opt_game_list` tests
  `finished_processing` after `parse_game`), so its "games processed"
  diagnostic is one higher and a parse error in that next game is still
  reported. pgnx stops after the game that triggers the condition. Output
  is identical.
- **`--keep-broken` evaluates the criteria against the partial replay.**
  A game whose replay fails is evaluated (length, end state, repetition,
  material) against the moves that did replay, then emitted in Source
  format wherever a matching/unmatched game would go, and still counted as
  broken (exit 2). The C tool with `--keepbroken` likewise evaluates what
  it managed to play, modulo the ordering notes above.
- **The summary line counts matched games.** `filter` prints
  `"N game(s) matched out of M."` where N counts every game that satisfied
  the criteria (including games held back by `--select-only`/
  `--skip-matching`, as `num_games_matched` does in C) and M is the number
  of games read. It is suppressed by `--quiet` and by `--suppress-matched`.
- **`--mark-matches FEN` uses pgnx's FEN rendering.** The C tool inserts
  `get_FEN_string(board)`; pgnx inserts `chess.Board.FEN()`. Both are
  standard six-field FEN; castling-right spelling for Chess960 positions
  may differ.
- **Chess960 `Variant` tag is not added.** `consistent_FEN_tags` in the C
  tool also adds `[Variant "chess 960"]` to a FEN game whose start position
  looks like a Chess960 setup but has no `Variant` tag. pgnx adds only the
  `SetUp "1"` tag. Flagged as a gap (no golden case); revisit with the
  Chess960 work.

## Positional matching and ECO (Phase 3)

Phase 3 is in progress. The FEN pattern matcher (`filter.PatternSet`, a
port of `fenmatcher.c`) is complete and was checked against the C binary
on one TWIC issue (1709 games) with eighteen `--fenpattern`/`--fenpatterni`
patterns covering literals, `?`, `!`, `A`/`a`, `m`, digits, `*`, `[...]`,
`[^...]`, material constraints and inverse forms; matched-game counts were
identical in every case. Known, intentional differences:

- **An inverse pattern with a material constraint keeps the constraint.**
  `add_fen_pattern` builds the colour-reversed pattern by truncating the
  original text at the scan pointer, which has already stepped past the
  `:` when a constraint is present; the reversed text therefore starts
  with `rank1:` and is rejected as "badly formed", so `--fenpatterni
  'pattern:constraint'` silently matches only the original orientation
  (verified with the C binary). pgnx adds the reversed pattern with the
  same constraint. Because the C tool parses the constraint of an inverse
  pattern with `both_colours` set, the constraint is already
  colour-symmetric, so this only ever adds matches of the mirrored
  position, never changes which boards the original orientation matches.
- **`m` is colour-neutral under inversion.** `reverse_fen_pattern` swaps
  the case of every letter, turning the not-a-pawn metacharacter `m` into
  `M`, which matches nothing. pgnx leaves `m` unchanged in the reversed
  pattern.
- **A malformed material constraint is an error.** The C tool logs the
  bad constraint and stores the pattern without one; `PatternSet.Add`
  returns the parse error and stores nothing. Malformed patterns are an
  error in both (the C tool logs and continues; pgnx reports through the
  CLI).

## Differential-testing findings (2026-08-07 run)

`scripts/difftest.sh` was run over every file in
`cmd/pgnx/testdata/golden/infiles/*.pgn` (56 files, default `san` format)
and over pgn-extract's own `eco.pgn` (2,014 games, ~12k lines) as a
larger real-world corpus. The full per-file table lived in an uncommitted
working-session report and was not preserved; to regenerate it, run
`for f in cmd/pgnx/testdata/golden/infiles/*.pgn; do scripts/difftest.sh "$f"; done`
(requires the C binary — see the script's `C_BIN` variable). Two findings
were fixed; the rest are the documented differences above.

- **Fixed:** a leading file-level comment with no tag section, no moves,
  and no result before it (e.g. `eco.pgn`'s opening `{ ... }` credits
  block, which precedes the first game's `[ECO ...]` tags with nothing in
  between) was parsed by pgnx as a phantom empty game (`Result: "*"`,
  default seven-tag-roster values) and emitted as spurious output. The C
  oracle (`grammar.c`'s `parse_game`) explicitly discards any prefix
  comment seen before a game's tag section and treats a "game" with no
  tags, no moves, and no result as nothing to emit. `pgn.parser.parseGame`
  now applies the same rule: a parsed unit with zero tags, zero moves, and
  an empty result is skipped rather than returned. Verified: pgnx's output
  over `eco.pgn` is now byte-identical to the C oracle's.
- **Open divergence — tags-only game (no moves, no result) is emitted by
  pgnx, discarded by the C oracle.** A "game" consisting of a complete tag
  section but no movetext and no result token — a stub record some
  real-world PGN collections contain, e.g. a placeholder entry with
  `[Event]`/`[Site]`/etc. filled in but the game itself never played out —
  is dropped entirely by pgn-extract (`grammar.c`'s `parse_game` requires
  at least one move or a result token, in addition to tags, before it will
  emit a game), but pgnx's parser has no such check for the tags-only case
  and emits a full seven-tag header plus a bare `*` result line. This is
  the mirror image of the comment-only phantom-game case fixed above
  (there: no tags, no moves, no result, discarded on both sides now; here:
  tags present, no moves, no result, discarded only by the C oracle).
  Repro:

  ```sh
  cat > tagsonly.pgn <<'EOF'
  [Event "Test"]
  [Site "?"]
  [Date "????.??.??"]
  [Round "?"]
  [White "?"]
  [Black "?"]
  [Result "*"]

  EOF

  # C oracle: emits nothing.
  pgn-extract -7 --quiet tagsonly.pgn

  # pgnx: emits the full seven-tag header plus "*".
  pgnx convert --quiet --seven-tag-roster tagsonly.pgn
  ```

  Not fixed in this pass — `pgn.parser.parseGame`'s existing skip rule
  (`len(g.Tags) == 0 && len(g.Moves) == 0 && g.Result == ""`) would need
  to be loosened to also skip when tags are non-empty but moves and result
  are both empty, which risks silently dropping legitimate tag-only
  records other callers may rely on parsing; flagged here for a
  deliberate decision rather than folded into this fix wave.
- **Open concern — invalid-UTF-8 bytes are corrupted to U+FFFD.** Several
  `odds.pgn` tag values contain raw Latin-1 bytes (e.g. `0xE7`, `0xE9` for
  "Fran\xe7ois", "Andr\xe9"). pgn-extract passes these bytes through
  untouched (it is byte-oriented). pgnx's lexer (`pgn/lexer.go`) reads
  input via `bufio.Reader.ReadRune`, which decodes UTF-8 and substitutes
  the replacement character U+FFFD for any invalid byte; the original byte
  is unrecoverable by the time a token's text is built. Fixing this
  requires switching the lexer's core read primitives from rune-oriented
  to byte-oriented (touching every token type: tag names, strings,
  comments, moves), which is more than a minimal, oracle-justified patch
  for Phase 1. Flagged here rather than fixed; revisit if golden/
  differential corpora surface more non-UTF-8 real-world PGN.
- **Script scope note, not a bug:** `difftest.sh` runs both binaries with
  no flags beyond `--quiet`/`-f`/`-o` (per its usage: `<input.pgn>
  [format]`). `testdata/golden/infiles/nested-comment.pgn` is designed to
  exercise `--nestedcomments`/`--nested-comments`; without that flag, both
  tools' lenient-recovery behavior differs (pgn-extract's per-token
  recovery still emits six empty games from the malformed movetext, while
  pgnx's replay drops the one game outright on the first illegal token).
  With `--nested-comments` passed to pgnx and `--nestedcomments` to
  pgn-extract — the flag combination the golden harness actually uses —
  output is byte-identical. Not a real divergence; the plain differential
  script simply doesn't thread per-file flags.

## Differential-testing findings (2026-10-29 run, Phase 2)

`scripts/difftest.sh` gained `PGNX_CMD`, `C_FLAGS` and `PGNX_FLAGS` so
filter runs can be diffed. The sweep described under "Filtering" above
surfaced three Phase 1 (`convert`) divergences; two were fixed with the
oracle as justification and one is documented.

- **Fixed:** unrecognised tags were emitted in each game's source order.
  pgn-extract's lexer assigns an unknown tag name an index the first time
  it is seen anywhere in the run (any game, matched or not, or a `-t`
  file), and `output.c` emits leftover tags by index — i.e. in run-wide
  first-seen order. On a Chess.com export where some games list
  `Tournament` before `CurrentPosition` and others after, the two tools
  disagreed. `output.TagRegistry` now records first-seen order; `convert`
  and `filter` share one registry per run, fed every parsed game (and the
  tag names in criteria files) before writing. The five tags pgn-extract
  predefines but leaves out of its default order (`BlackFideId`,
  `HashCode`, `MatchLabel`, `TotalPlyCount`, `WhiteFideId`) are emitted
  ahead of unknown tags, in that order, as their fixed indices dictate.
- **Fixed:** `convert` emitted `[Result "1/2"]`, `[Result "?"]` and
  invalid Result tags (e.g. `"1:0"`) unchanged. The C parser's
  `check_result` (`grammar.c`) rewrites `"1/2"` to `"1/2-1/2"` and fills an
  absent, empty or `"?"` tag from the terminating token, and `play_moves`
  (`apply.c`) replaces any remaining non-PGN value with `"*"`. The output
  writer now applies the same normalisation to the Result roster tag;
  `filter` applies it before tag matching as well.
- **Open divergence — a game with moves but no result token.** When a
  game's movetext is not terminated by a result token, pgn-extract reports
  "Missing result" and, if the game is the last in the file, emits nothing
  for it; if another game follows, it emits the tag section with
  `[Result "*"]` and a bare `*` with the moves discarded. pgnx emits the
  moves followed by `*` in both cases. The C behaviour loses data and
  looks like a parser-recovery artefact rather than a design choice, so it
  is documented rather than copied. Repro:

  ```sh
  printf '[Event "first"]\n\n1. e4 e5 2. Nf3\n\n[Event "second"]\n\n1. d4 *\n' > noresult.pgn
  pgn-extract --quiet noresult.pgn   # first game: tags + "*" only
  pgnx convert --quiet noresult.pgn  # first game: "1. e4 e5 2. Nf3 *"
  ```
