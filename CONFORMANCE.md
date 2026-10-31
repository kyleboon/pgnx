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
  `FEN`, `FENPattern` and `FENPatternI` lines parse as positional
  criteria (the pattern forms with the rest of the line as a label) and
  are routed to the positional matcher (see Phase 3 below).
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
  may differ. (Parsing is not affected: `KQkq` in an input FEN resolves to the
  outermost rook on the back rank, as `find_rook_starting_position` does.)
- **Chess960 `Variant` tag is not added.** `consistent_FEN_tags` in the C
  tool also adds `[Variant "chess 960"]` to a FEN game whose start position
  looks like a Chess960 setup but has no `Variant` tag. pgnx adds only the
  `SetUp "1"` tag. Flagged as a gap (no golden case); revisit with the
  Chess960 work.

## Positional matching and ECO (Phase 3)

`pgnx filter` ports pgn-extract's positional criteria (`apply.c`
`play_moves`/`position_matches`, `fenmatcher.c`, the `-x` reader in
`moves.c`), move-sequence matching (`moves.c` `check_textual_variations`)
and their `--startply`/`--matchplylimit`/`--wtm`/`--btm`/`--markmatches`/
`--addlabeltag` modifiers; `convert` and `filter` port ECO classification
and ECO-split output (`eco.c`). Every in-scope target of the C test suite
is a golden row, and a differential sweep of fifteen flag combinations
(`-x`, `-v`, `-P -v`, `--vanywhere`, `-Tf`, `--fenpatterni`, `-H`, `-t`
with FEN lines, `-e`, `-e -7`, `-e -TeB1`, `--startply` with `-Tf`,
`--matchplylimit` with `-x`, `--wtm`, `--addlabeltag`) over the golden
inputs and one 8,005-game TWIC issue produced byte-identical output
apart from the pre-existing classes listed under the 2026-10-30 findings
below. Known, intentional differences:

- **pgnx replays past the search depth.** `play_moves` stops applying
  moves once the ply count passes the positional search depth
  (`--matchplylimit`, or the length of the longest `-x` line) without a
  match, so an illegal move beyond that depth in a non-matching game is
  never noticed by the C tool. pgnx replays every game in full before
  evaluating criteria: such a game is logged as broken and makes the
  exit status 2. Output is unaffected (the game is emitted nowhere
  unless `--keep-broken`); the matched and `-n` outputs were confirmed
  identical on the TWIC issue with `--matchplylimit 20`.
- **Malformed positional input is a usage error.** A FEN pattern that is
  "badly formed" is logged and dropped by the C tool, which then, with
  positional matching switched on but nothing to match, selects no
  games; a malformed material constraint is logged and the pattern kept
  without it; a `-v` line containing only move numbers becomes an empty
  sequence that every game matches. pgnx rejects all three with exit
  status 1 (`PatternSet.Add`, `ParseSequence`'s `ErrNoMoves`), naming
  the flag or file line. An unreplayable `-x` line, a non-hexadecimal
  `-H` value and an unreadable ECO file are fatal in both tools.
- **Sequence matching is textual on the moves as written**, whatever
  the output format. `-v`/`--sequence` compares the move text from the
  source (check glyphs normalised to a single `+`, `!`/`?` glyphs
  dropped), not the canonical SAN that `convert` emits, because the C
  tool runs `check_textual_variations` before its output-time rewrite.
  `Ngf3` in a sequence therefore matches only a game that wrote `Ngf3`,
  even though the `san` output of that game shows `Nf3`. This is parity,
  recorded here because it is easy to mistake for a bug.
- **No implicit ECO file.** `-e` with no file name uses `$ECO_FILE` or
  `eco.pgn` in the current directory; `--eco-file` requires a path.
- **ECO diagnostics differ.** The C tool logs every "ECO hash collision"
  (two lines in the ECO file reaching the same position at the same
  ply) and each line it fails to replay as the file is read. pgnx keeps
  the first of two colliding lines silently (the same line the C tool
  keeps) and reports unreplayable lines as one count on the log writer
  (`skipped N ECO lines that could not be replayed`), suppressed by
  `--quiet`.
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
- **Fixed — invalid-UTF-8 bytes were corrupted to U+FFFD.** Several
  `odds.pgn` tag values (and many TWIC team/player/site tags) contain raw
  Latin-1 bytes (e.g. `0xE7`, `0xE9`). pgn-extract is byte-oriented and
  passes them through; pgnx's lexer used `bufio.Reader.ReadRune`, which
  substituted U+FFFD. `pgn/lexer.go` now reads and peeks single bytes
  (`ReadByte`) and appends bytes to token text, so Latin-1 and UTF-8 alike
  pass through tag values and comments unchanged; bytes >= 0x80 outside
  strings/comments are skipped as unknown characters, as before. The
  scanner still streams one game at a time. Output wrapping counts bytes
  (`len`), as the C tool does. Covered by `pgn/bytes_test.go`. Tag output
  must not re-escape these bytes (the `%q` quoting in `output/output.go`
  is tracked separately); with raw quoting `difftest.sh` over `odds.pgn`
  and several Latin-1 TWIC issues is clean.
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

  Narrow exception, matching the oracle: if such a game's moves do **not**
  replay (the C tool never replays them, so it never notices), pgnx drops
  the moves and emits the tag section with the Result tag as the movetext,
  exactly as pgn-extract does, and reports the game as a problem (exit 2).
  A TWIC double forfeit whose whole movetext is `0-0` hits this: `0-0` is
  lexed as kingside castling, there is no result token, and the C tool
  emits a bare `*` game (see the stray-token entry below). Without the
  exception pgnx rejected the game and emitted nothing. `--keep-broken`
  keeps the moves (Source format) instead.

## Stray movetext tokens (beads `pgnx-twic-straytoken`)

About 45 TWIC games carry non-move words in the movetext: a lone `-`,
`+/-` / `-/+` as the whole movetext of a forfeit, a bare piece letter
(`26. N 1/2-1/2`), `-O`, and `0-0` as a double-forfeit "result". pgnx used
to treat each as a move, fail the replay and emit nothing for the game
(exit 2). The decision is to **match the oracle**:

- **Fixed:** the lexer now classifies a word as a move only if it "seems
  valid" (a port of `move_seems_valid`, `decode.c`, used by `lex.c`); any
  other word, and a single `-` that does not begin the null move `--`
  (`lex.c`, DASH case, "Single '-' not allowed"), is a stray token. As in
  the C lexer (`Unknown move text N.`) the parser discards it and keeps the
  game; the movetext simply continues. `+` and `/` were already ignored.
  The token is logged as a warning (`game n: warning: ... skipped
  unrecognised movetext token "N"`; wording is not held to parity) and the
  game is emitted, but it counts as a problem, so the run exits 2 (the C
  tool exits 0). `Game.SkippedTokens` exposes them to library users.
- **Fixed:** a game with no moves is printed with its (normalised) Result
  tag as the movetext, not the result token and not a default `*`
  (`output.c`, "Take account of a possible zero move game"). So `+/-`
  movetext with `[Result "+/-"]` gives the tag section with `[Result "*"]`
  and a bare `*`; `26. N 1/2-1/2` with `[Result "1-0"]` gives a bare `1-0`.
- **Fixed (narrowly):** `0-0` as the entire movetext, see the exception
  added to the missing-result entry above.
- **Verified:** the stray-token games of `twic943`, `twic1041`, `twic1051`
  and `twic1110` are byte-identical to the oracle; golden test
  `TestStrayTokensConformance` (`cmd/pgnx/testdata/stray`) covers `+/-`,
  a lone `-`, bare piece letters, and `0-0`. `ff` is not a stray token:
  both tools take it as a pawn move and reject the game (C: "Failed to
  make move").
- **Open divergence — `-1` / `-2` and other digit-after-dash tokens.** In
  `1. e4 e5 -1 2. Nf3 1/2-1/2` the C lexer discards `-` and then yields
  the move number `1`; two move numbers in a row end `parse_move_list`
  (`grammar.c`), the game is reduced to "Missing result" (tags only), and
  `skip_to_next_game` restarts at the next move, producing a second,
  tagless game `1. Nf3 1/2-1/2`. pgnx skips the `-` and parses one intact
  game `1. e4 e5 2. Nf3 1/2-1/2`. This is a recovery artefact of the C
  grammar (a phantom game and lost moves), so it is documented, not
  copied. Repro:

  ```sh
  printf '[Event "x"]\n[Result "1/2-1/2"]\n\n1. e4 e5 -1 2. Nf3 1/2-1/2\n' > dash1.pgn
  pgn-extract --quiet dash1.pgn   # tags + "1/2-1/2", then a tagless "1. Nf3 1/2-1/2" game
  pgnx convert --quiet dash1.pgn  # "1. e4 e5 2. Nf3 1/2-1/2"
  ```
- **Open divergence (unchanged):** when a file's last game is only stray
  tokens (movetext `-`, nothing after), the C tool stops at EOF and emits
  nothing; pgnx emits the tag section and `*`. This is the tags-only
  divergence above; mid-file the two agree.
- **Known minor difference:** the C lexer gathers a move word from its own
  narrower character set, so `Nf3Z` is `Nf3` plus junk there; pgnx gathers
  the whole word and discards it.

## Differential-testing findings (2026-10-30 run, Phase 3)

The sweep described under "Positional matching and ECO" above ran
`scripts/difftest.sh` with fifteen `C_FLAGS`/`PGNX_FLAGS` combinations
over every golden input except `nested-comment.pgn` (which needs the
per-file flag the script does not thread; see the 2026-08-07 note) and
over `twic1500.pgn` (8,005 games). One Phase 3 divergence was fixed; the
remaining diffs are pre-existing classes already documented here.

- **Fixed:** the colour-reversed pattern added by `--fenpatterni` was
  unlabelled, so `--addlabeltag` added no `MatchLabel` tag to a game
  matched in the mirrored orientation. `add_fen_pattern` receives the
  empty string as the label from the `--fenpatterni` flag and labels the
  reversed pattern `label + "I"`, i.e. `"I"`, so the C tool emits
  `[MatchLabel "I"]` for such games. `PatternSet.Add` now always labels
  the inverse `label+"I"`. A `FENPatternI "pattern"` criteria line with
  no label is different: `taglines.c` passes a NULL label and the
  reversed pattern gets `""`, so no tag is added; `PositionCriteria.
  AddCriterion` reproduces that. Both forms were confirmed against the C
  binary on `petrosian.pgn` and the TWIC issue.
- **Fixed (`pgnx-w1j`), surfaced by `-e -7`:** `--seven-tag-roster`
  used to drop the `Variant`/`SetUp`/`FEN` tags of a game that starts
  from a FEN, leaving the movetext unreplayable. Following `output.c`
  `print_algebraic_game`, a game carrying a `FEN` tag now gets `Variant`,
  `SetUp` and `FEN` (each if present, in that order) after the seven
  roster tags and after any ECO extras. `SetUp` without `FEN` is not
  emitted. `test-promotion-in.pgn` and `test-setup-in.pgn` are now
  byte-identical under `-7` (with and without `-e`); `odds.pgn` still
  differs for Latin-1 bytes and null moves, documented above. The
  separate gap that `consistent_FEN_tags` does not add a Chess960
  `Variant` tag is unchanged.
- Also seen, already documented above: NAG/comment interleaving
  (`test-nagcomments.pgn`), comments across a variation boundary
  (`test-splitvariants.pgn`) and Latin-1 tag bytes (`odds.pgn`).

## Differential-testing findings (2026-10-31 TWIC corpus run)

`scripts/twicdiff.sh` runs both tools over every file of a directory in
parallel (`convert -f san`, byte comparison, a `.diff` kept per divergent
file) and is the repeatable in-the-wild regression check behind the
beads `pgnx-twic` epic. The corpus is the 749 TWIC issues 920–1668
(about 3.6M games; local only, `twic/` is gitignored). Baseline before
this epic, 2026-10-29: 588/749 issues identical, 161 divergent, 928
games rejected by pgnx. After the five sibling fixes (Latin-1 bytes,
Chess960 castling rights, tag quoting, stray movetext tokens,
black-to-move numbering): 740/749 identical, 9 divergent, 227 rejected,
70 stray-token warnings. Two new classes surfaced and were fixed; the
run then stands at 744/749 identical, 5 divergent, 226 rejected, 70
warnings (663 s wall, 8 workers).

- **Fixed — `convert` did not add `SetUp "1"` to a FEN game.** grammar.c
  `consistent_FEN_tags` runs on every parsed game before `apply_move_list`,
  whether or not criteria are in force, and silently adds the missing
  `SetUp` tag ("such a common problem that it makes more sense just to
  silently correct it"). pgnx did this only inside `filter.Evaluate`, so
  `pgnx filter` matched the oracle and `pgnx convert` did not (TWIC 1267,
  1393, 1426: a Chess960 event, two games tagged with the standard start
  FEN, one adjourned-game fragment). `filter.ConsistentFENTags` is now
  exported and `convert` calls it before replaying; following the C tool
  it also drops a game with `SetUp "1"` and no FEN tag, or an unparsable
  FEN, which pgn-extract emits nowhere (pgnx logs it and exits 2; the C
  tool logs "Missing FEN Tag to accompany SetUp Tag" and exits 0).
  `TestConvertAddsSetUpToFENGame`/`TestConvertDropsSetUpWithoutFEN`.
  The separate gap that `consistent_FEN_tags` also adds
  `[Variant "chess 960"]` to an untagged Chess960 start is unchanged.
- **Fixed — a pawn move to the last rank with no promotion piece.**
  `69. Kf5 b1 0-1` (TWIC 970) is accepted by the C tool as `b1=Q+`:
  map.c `determine_move_details` reclassifies a `PAWN_MOVE` whose
  destination is the last rank as a promotion and, "since the move
  string doesn't tell us", assumes a queen. pgnx rejected the game.
  `replay.decodeSAN` now keeps the queen candidate when no promotion
  piece is written, for SAN, captures and bare long-algebraic pawn moves
  alike; `san`, `lalg` and `uci` output of such a move matches the
  oracle (`b8=Q`, `b7b8Q`). `TestDecodeSAN`.
- **Remaining divergences, all one documented class.** The five issues
  still differing (TWIC 1071, 1182, 1196, 1260, 1469) are games whose
  movetext ends in a malformed result — `-` (with `[Result "-"]`),
  `1-O`, `00-1`, `0-2` — or hits two move numbers in a row after stray
  tokens (`34. R R 35.`). The C grammar ends the move list, reports
  "Missing result" and emits the tag section with the Result tag as the
  movetext (plus, for the consecutive-move-number case, a phantom
  tagless game); pgnx keeps the moves and emits them with `*`, or
  rejects the game when the moves do not replay. See the missing-result
  entry under the 2026-10-29 findings and the `-1`/`-2` entry under
  stray tokens; these are recovery artefacts and are documented, not
  copied.
- **Remaining rejections are genuine.** The games pgnx still rejects are
  rejected by the C tool too (the outputs are identical), chiefly `--`
  null moves on the main line, illegal castling and impossible piece
  moves, plus the stray-token games whose skipped word shifts the move
  colours.
