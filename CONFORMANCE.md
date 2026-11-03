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
  requested. pgnx's `convert --keep-broken` (and `filter --keep-broken`)
  always emits broken games in Source format (SAN exactly as parsed)
  regardless of `-f`: the moves after the failure have no replay detail
  (`replay.Result`), which every other format needs. The rule lives in
  the writer: `output.Writer.WriteGame` given no replay renders Source
  with its other options (line width, roster, stripping) unchanged, and
  the CLI writes a broken game without its partial replay.
- **`validate`/`convert` I/O error lines go to stderr/`--log`, not
  stdout.** Unreadable input files are reported on the log writer
  (stderr by default, or `--log FILE`); they are not folded into
  `validate`'s findings stream on stdout, so `--summary` does not
  suppress them.

## Parser data-model differences

- **NAG/comment interleaving and comments after a variation's closing
  paren are preserved** since 2026-11-03: `pgn.Move` keeps one ordered
  list of the tokens that followed the move (`Annotations`: NAGs,
  comments, variations) in source order, and output walks it, so
  `$1 { c } $2` and `(2. Nf3) { c4 }` round-trip as the C tool writes
  them. See "Ordered annotations" below for the details and the
  remaining differences in that area.
- **A NAG after a variation is kept.** pgn-extract's grammar ends a
  move's annotations at its variations (`move = move_unit NAG_list
  variant*`), so `1. e4 (1. d4) $2 { b } e5 *` makes it abandon the
  movetext at `$2`: it reports "Missing result", emits the game with
  only its tags and `*`, and reads `{ b } e5 *` as a tagless second
  game whose first move fails. pgnx keeps the NAG on the move in place
  and emits `1. e4 (1. d4) $2 { b } 1... e5 *`, a leniency of the same
  kind as the stray-token recovery below.

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
- **A game with `SetUp "1"` and no FEN tag (or an unparsable FEN) is
  emitted nowhere, not even to `--unmatched`.** In the C tool a
  `consistent_FEN_tags` failure only makes the game a non-match: the main
  output never sees it, but the non-matching branch of `deal_with_game`
  (`grammar.c`) still replays it from the standard start and writes it,
  `[SetUp "1"]` and all, to the `-n` file, exit 0:

  ```sh
  printf '[Event "a"]\n[SetUp "1"]\n\n1. e4 *\n' | pgn-extract -TpZZZ -n/dev/stdout -o/dev/null --quiet
  # logs "Missing FEN Tag to accompany SetUp Tag" and prints the game
  ```

  pgnx normalises every game once, before replay, for every command
  (`pipeline.Normalise`, ADR 0002), and a game that step rejects is
  Rejected in the glossary's sense: logged, emitted nowhere (matched,
  `--unmatched` and `--keep-broken` alike), exit 2. `convert` and
  `filter` therefore agree with each other; the `-n` emission of a game
  whose tags the C tool itself called inconsistent is a recovery artefact
  and is not copied. `TestFilterRejectsSetUpWithoutFEN`.
- **Verified: an invalid Result tag is rewritten to `*` on the unmatched
  path too.** `filter.Evaluate` applies the `apply.c` `play_moves` rule
  (an unrecognised Result tag becomes `*`) after the tag criteria have
  seen the tag as written, whether or not they passed (`pgnx-deepen.3`,
  commit 1b36160). Checked against the oracle on 2026-11-02: the C tool's
  non-matching branch replays the game before writing it to `-n`
  (`check_move_validity` repeats the rule), so `-n` carries `*` as well,
  and the tag criteria see the raw value (`-Tr1:` matches, `-Tr*` does
  not); with no criteria, and with a `*` movetext result, likewise:

  ```sh
  printf '[Event "x"]\n[Result "1:0"]\n\n1. e4 1-0\n' | pgn-extract -TpZZZ -n/dev/stdout -o/dev/null --quiet
  # prints the game with [Result "*"] and the 1-0 token, logging
  # "Inconsistent result strings * vs 1-0"
  printf '[Event "x"]\n[Result "1:0"]\n\n1. e4 *\n' | pgn-extract -TpZZZ -n/dev/stdout -o/dev/null --quiet
  # prints the game with [Result "*"]
  ```

  `pgnx filter --player ZZZ --unmatched -`, `pgnx convert`, `--result 1:`
  and `--result '*'` give byte-identical output for the same inputs.
  `TestEvaluateInvalidResultTagBecomesStar`.
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
  looks like a Chess960 setup but has no `Variant` tag. `pipeline.Normalise`
  adds only the `SetUp "1"` tag. Flagged as a gap (no golden case); revisit with the
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

## Output transforms (Phase 2b: convert transforms, `pgnx split`)

`convert`, `filter` and `split` port pgn-extract's output-side rewrites
(output.c `format_game`/`print_items_following_move`, apply.c
`rewrite_moves`/`drop_plies_from_start`/`shannonEvaluation`, map.c
`init_hashtab` for the HashCode tag, grammar.c `select_output_file` for
`-#`). Every in-scope target of the C test suite is a golden row or
harness function (test-plycount, test-addhashcode, test-plylimit,
test-FF, test-fencomments, test-evaluation, test-nochecks, test-notags,
test-linenumbers, test-dropply, test-hash), and a differential sweep of
nineteen flag combinations over the 6,083-game TWIC issue 1200
(`--plycount --totalplycount --addhashcode`, `--fencomments`,
`--hashcomments`, `--evaluation`, `-F`, `--commentlines`, `--commentlines
--fencomments -w40`, `--notags` in SAN and EPD, `--nochecks` in SAN and
ELALG, `--linenumbers`, `--plylimit 7` with and without `-V -C`,
`--dropply 6`, `--dropply -4 --plycount`, `--dropply 2 -Wfen`,
`-#500,3`) was byte-identical. Known, intentional differences:

- **A game too short for `--drop-ply` is reported.** The C tool's
  rewrite fails for a game with fewer plies than `--dropply` asks to
  drop (or to keep, for a negative count); `format_game` then emits
  nothing, silently, while the run's summary still counts the game as
  matched. pgnx Rejects the game with a reason on the log (`cannot drop
  N plies from a game of M`), emits it nowhere, and exits 2. Output is
  identical. Three games of TWIC 1200 (0- and 1-ply games) hit this.
  Under `--keep-broken` both tools emit the game untouched by the drop
  (`rewrite_game` keeps the board); pgnx also logs it and exits 2.
- **`--drop-before` reads only a move's leading comments**, as
  `find_matching_comment` reads only the C tool's `comment_list`: a
  comment written after a NAG (`e5 $1 { c2 }`) or after a variation
  drops nothing in either tool (`pgn.Move.LeadingComments`, since
  2026-11-03; before that pgnx matched every comment on the move).
- **Kept broken games count and annotate their typed moves.** With
  `--keep-broken`, the C tool's PlyCount, TotalPlyCount, `--evaluation`
  and `--fencomments` cover only the plies that replayed (the rest
  become a comment); pgnx counts every parsed move and, emitting the
  game in Source format without a replay (the `--keep-broken` entry
  above), adds no generated comments. HashCode sums the plies that
  replayed in both tools.
- **`--drop-before` counts absolute plies, as the C tool does.** The
  number of plies to drop is taken from the board's move number after
  the commented move (`(move_number - 1) * 2`, plus one for Black to
  move), which for a game starting from a FEN counts from move 1, not
  from the game's first move. So `--drop-before` on a FEN game can drop
  more plies than lie before the comment, or reject the game as too
  short. Mirrored deliberately (`transform.dropPlies`); it is what the
  C tool outputs.
- **A HashCode tag is always recomputed.** `format_game` rewrites a
  HashCode tag the game already carries whether or not `--addhashcode`
  was given; pgnx does the same (`transform.Apply` with zero options).
  The value is the C tool's: the low 32 bits of the sum of its weak
  hash (`chess.Board.WeakHash`, a port of map.c's table) over every
  main-line position after a move, taken before any plies are dropped.
- **`-Fpattern`, `--fencommentformat` and `--quiescent` are not
  ported.** `--final-fen` is the plain `-F` (a quoted FEN comment after
  the last move); the C form that replaces a comment matching a
  pattern with the FEN, the format string for FEN comments, and the
  quiescence threshold that lets `--plylimit` run on past the limit
  until a quiet move have no pgnx surface. The C test suite's
  test-FF-text target (`-Fdiagram`) is excluded from the golden harness
  on this basis.
- **`--split-variants` splits once, then transforms each game.**
  `--splitvariants` (grammar.c `split_variants`) is ported as
  `transform.SplitVariations`: the main line stripped of variations,
  then one game per variation, depth first, with the comments that
  followed a variation's closing paren moved before the preceding
  move's NAGs, a variation's prefix comment onto the move before the
  branch, a Result tag of `*` and the variation's own result token or
  `*`. test-splitvariants is a golden row, and a differential of the
  flag with `--ply-count --total-ply-count --add-hash-code`,
  `--fen-comments`, `--hash-comments`, `--evaluation`, `--final-fen`,
  `--ply-limit 3`, `--no-comments`, `--no-nags`, `--comment-lines`,
  `--split-depth 1` and the lalg/elalg/epd formats over every golden
  input with a variation was byte-identical. The C tool, though, calls
  `format_game` (which rewrites the game) on the *same* `Game` for
  every split game, so its other rewrites compound: `--linenumbers`
  prepends its comment once more per game emitted (the sixth game of a
  split carries six copies), `--dropply N` drops N further plies from
  each game in turn (and loses the variations of the plies dropped),
  and `--dropbefore` with `--splitvariants -o` crashes. pgnx splits the
  game as parsed and then runs the transforms over each emitted game
  independently, so each carries the line-number comment once, has its
  own first N plies dropped and its own `--drop-before` comment looked
  for. What the C tool does share is mirrored: every split game's
  HashCode is the main line's (`cumulative_hash_value` is summed when
  the game is replayed for matching), while PlyCount and TotalPlyCount
  count the emitted game. UCI output does not split (`-Wuci` turns
  `keep_variations` off, and `output_game` splits only with it on;
  the C tool makes `-Wuci --splitvariants`, in that order, a usage
  error, pgnx never does), and `--split-variants` with
  `--no-variations` is a usage error, as `-V` with `--splitvariants`
  is. `pgnx split --games N` counts source games, as `-#N` counts
  matched games (`select_output_file`), so a game's split parts stay
  in one file. Under `--keep-broken` every game split from
  a broken game is emitted in Source format; a variation that does not
  replay (which the C tool fails the whole game for, with or without
  the flag, and pgnx tolerates) is reported, emitted nowhere, and under
  `--keep-broken` emitted in Source format. Note that the C tool's
  `--splitvariants` reads a following numeric argument as its depth, so
  `--splitvariants -7` swallows `-7`; `-7 --splitvariants` is the
  equivalent of `--split-variants --seven-tag-roster`.
- **Usage errors instead of warnings.** `--no-tags` with
  `--seven-tag-roster` or `--roster`, a negative `--ply-limit`, a
  missing player file, and `split` with `--output` or `--eco-split`
  exit 1; the C tool exits 1 for the first three and warns-and-ignores
  `-o` with `-#`.
- **`--fen-comments` in UCI output.** `-Wuci` disables comments, NAGs,
  variations, move numbers and check suffixes in the C tool, but not
  the generated FEN/hash/evaluation comments, which it still emits; pgnx
  mirrors this.
- **`--ply-limit` in FEN output.** `print_FEN_move_list` tests the limit
  once, against the starting ply: a game that starts past it lists no
  positions, one within it lists every position; EPD output ignores the
  limit altogether. Mirrored.
- **`--no-checks` on Source output.** The C tool cuts the move text at
  its first `+` (failing that, `#`) and drops whatever followed. pgnx's
  Source format keeps `!`/`?` glyphs on the move text rather than
  turning them into NAGs (the C lexer does), so `Qh4#!!` becomes `Qh4`
  in both tools, but the C tool also emits `$3` where pgnx emits
  nothing; that is the pre-existing glyph difference, not a
  `--no-checks` one.

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
- **Intentional, behind a parser flag — tags-only game (no moves, no
  result).** A "game" consisting of a complete tag section but no movetext
  and no result token — a stub record some real-world PGN collections
  contain, e.g. a placeholder entry with `[Event]`/`[Site]`/etc. filled in
  but the game itself never played out — is never emitted by pgn-extract
  (`grammar.c`'s `parse_game` returns `current_symbol != EOF_TOKEN`, so a
  game that reaches end of input without a result token is never dealt
  with). pgnx's parser used to yield it, and the CLI emitted a full
  seven-tag header plus a bare `*`. Decided 2026-11-02 (ADR 0002, beads
  `pgnx-tagsonly-game`): the parser skips it behind
  `pgn.ParserOptions.SkipTagsOnly`, which all four commands set, so the
  CLI's game counts and numbering match the oracle; the library default
  keeps the record. The rule is "tags and an empty movetext" (no moves,
  no result token, nothing the parser discarded, no comment), or any
  game with no moves and no result token that ends at end of input; a
  mid-file zero-move game whose movetext held a stray token or a comment
  is still emitted, as the C tool emits it (the `+/-` forfeit in
  `TestStrayTokensConformance`). Mid-file, a stub followed directly by
  the next game's tags is not a game in either parser: both read tags
  across blank lines, so the stub's tags run on into the next game's
  (last occurrence wins) and the two outputs are identical.
  `TestScannerSkipTagsOnly`, `TestTagsOnlyGameSkipped`. Repro:

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

  # Both emit nothing and exit 0.
  pgn-extract -7 --quiet tagsonly.pgn
  pgnx convert --quiet --seven-tag-roster tagsonly.pgn

  # Mid-file the stub's tags merge into the next game in both tools:
  # one game, Event "u", White "Ghost", Result "1-0".
  printf '[Event "t"]\n[White "Ghost"]\n[Result "1-0"]\n\n[Event "u"]\n\n1. d4 *\n' > stub.pgn
  pgn-extract --quiet stub.pgn
  pgnx convert --quiet stub.pgn
  ```
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
  must not re-escape these bytes (every tag line now goes through
  `Writer.writeTag`/`quoteTagValue` in `output/output.go`, which escapes
  only a bare `"`; no Go `%q` remains); with raw quoting `difftest.sh`
  over `odds.pgn` and several Latin-1 TWIC issues is clean.
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
  (`apply.c`) replaces any remaining non-PGN value with `"*"`. The first
  two rules are `pipeline.Normalise`, which every command runs once per
  game before replay (ADR 0002); the third is the first step of the
  criteria stage (`filter.Evaluate`, which `convert` runs with no
  criteria), after tag criteria have seen the tag as written, as in C.
  The output writer trusts the tags it is given and only supplies the
  roster default `"*"` for an absent tag, so every format now prints the
  same Result: until 2026-11-02 the writer normalised the SAN roster tag
  at render time while the EPD `c1` comment printed the raw tag
  (`[Result "?"]` with a `*` token gave `c1 ?;`, the oracle gives
  `c1 *;`). `TestConvertEPDResultNormalised`,
  `TestConvertRosterResultNormalised`.
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
- **Fixed:** a game with no moves is printed with its Result tag (as
  normalised by the pipeline) as the movetext, not the result token and not a default `*`
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
- **Fixed (2026-11-02):** when a file's last game is only stray tokens
  (movetext `-`, nothing after), the C tool stops at EOF and emits
  nothing; pgnx used to emit the tag section and `*`. The tags-only skip
  (`SkipTagsOnly`, see the tags-only entry above) now covers any game
  with no moves and no result token that ends at end of input, so pgnx
  emits nothing either, and logs no warning for the discarded token
  (the C tool logs "Single '-' not allowed" but exits 0). Mid-file the
  two agree as before: such a game is emitted as a zero-move game.
  `TestTagsOnlyGameSkipped`.
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
  differs for Latin-1 bytes (null moves have since been ported; see
  "Null moves" below). The
  separate gap that `consistent_FEN_tags` does not add a Chess960
  `Variant` tag is unchanged.
- Also seen, since fixed or documented above: NAG/comment interleaving
  (`test-nagcomments.pgn`), comments across a variation boundary
  (`test-splitvariants.pgn`, now a golden row under `--split-variants`)
  and Latin-1 tag bytes (`odds.pgn`).

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
  FEN, one adjourned-game fragment). The rule now lives in
  `pipeline.Normalise`, which `convert`, `filter` and `validate` all run
  on every game before replaying it (ADR 0002; at first it was a
  `filter.ConsistentFENTags` call that only `convert` made). Following
  the C tool's main output it also rejects a game with `SetUp "1"` and no
  FEN tag, or an unparsable FEN: pgnx logs it and exits 2, emitting it
  nowhere; the C tool logs "Missing FEN Tag to accompany SetUp Tag",
  exits 0, and would still write the game to a `-n` file (see the
  Filtering section). `TestConvertAddsSetUpToFENGame`/
  `TestConvertDropsSetUpWithoutFEN`/`TestFilterRejectsSetUpWithoutFEN`.
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

## Replay exposes typed facts (2026-11-02, beads `pgnx-deepen.2`)

`replay.Position` now carries the ply number (`PlyNumber`, pgn-extract's
`play_moves` count: the one-based number of the move about to be played)
and `replay.MoveInfo` records en passant; output, filter and eco read
those instead of re-parsing the FEN string. One parity fix fell out:

- **Fixed — a FEN tag with a fullmove field of 0.** apply.c
  `new_fen_board` clamps a move number below 1 to 1, so the oracle
  numbers such a game from `1.` and its `-Wfen` output prints `... 6 1`
  for the start position. pgnx numbered the movetext from 1 already
  (output clamped on its own) but emitted the FEN with `0` and counted
  plies from -1. The clamp now lives where the C tool has it, on the
  replay's starting board, so every consumer agrees.
  `TestFullmoveClampedInFEN`, `TestFullmoveZeroFEN`.

## One pipeline per run (2026-11-02, beads `pgnx-deepen.4`)

`pipeline.New(Config).Process(g)` now runs pgn-extract's `deal_with_game`
sequence (normalise, replay, broken-ness against `--keep-broken`, ECO
classification, criteria) for `convert` and `validate`; `pgnx-deepen.5`
routed `filter` through it as well (see below). No output changed. Two
notes for triage:

- `validate` reports exactly what `convert` would fail on: a game
  Normalise rejects, a replay failure, or a game with no result token
  whose moves were dropped (a warning in `convert`, exit 2 there too).
  Before, `validate` replayed the original moves and reported the illegal
  move itself; now it reports the dropped moves. Both are findings.
- The criteria stage runs in `validate` and `convert` with no criteria,
  so the `apply.c` Result rewrite (an invalid tag becomes `*`) is applied
  before anything is written, exactly as for `filter`; `validate` writes
  nothing, so it is unobservable there.

## One run struct per command (2026-11-02, beads `pgnx-deepen.5`)

`convert`, `filter` and `validate` now share one per-game sequence in
`cmd/pgnx/run.go` (observe tags, `pipeline.Process`, route, write) and
one setup helper; `filter`'s private copies of the replay and ECO stages
are gone, and its route only sends the outcome through
`filter.Selector`, `--suppress-matched` and `--unmatched`. No output
changed: `TestFilterNoCriteriaEqualsConvertOverGoldenInputs` pins
byte-identical output, diagnostics and exit codes for `convert` and
`filter` with no criteria (with and without `--keep-broken`) over every
golden input, the golden suite is unchanged, and a filter-mode
differential (`PGNX_CMD=filter scripts/difftest.sh`, no criteria,
`-TpCarlsen` and `--checkmate`) over 21 TWIC issues showed no
divergence.

## Epic corpus result (2026-11-02, beads `pgnx-deepen`)

On the epic's integration tip, the full TWIC convert differential
(`scripts/twicdiff.sh`, SAN, 749 issues) reproduced the 2026-10-31
baseline exactly: 744/749 identical, the same five divergent issues
(twic1071, twic1182, twic1196, twic1260, twic1469: the documented
"missing result" class), 226 rejected games and 70 warnings with
identical per-file counts; and filter mode (`-TpCarlsen`,
`--checkmate`, and `-n` against `--unmatched`) was byte-identical over
twic1000, twic1250 and twic1500.

## Null moves (2026-11-03, beads `pgnx-nullmoves`)

`--allownullmoves` and `--odds` are ported: `pgnx convert`, `filter`,
`split` and `validate` take `--allow-null-moves`, and `filter` takes
`--odds`. The golden targets `test-allownullmoves` and `test-odds` are
rows in `conformance_test.go`, and the 39 local TWIC issues that contain
a main-line `--` (the "null moves on the main line" rejection class of
the 2026-10-29 baseline) are byte-identical to the C tool under the
flag, in every format and with every generated comment.

- **What a null move does to the board** follows `apply.c` `apply_move`,
  which skips `make_move` for a `NULL_MOVE` and only flips the side to
  move (advancing the move number after Black): the en passant square,
  the halfmove clock and the castling rights are left exactly as they
  were, so `1. e4 --` reaches `... w KQkq e3 0 2`. `chess.NullMove` and
  `Board.Apply` reproduce this; the FEN, EPD, `--fencomments`,
  `--hashcomments`, `--evaluation`, `HashCode`, `PlyCount` and
  `--dropply` outputs were each checked against the C binary.
- **The stale en passant square** that a null move leaves with the
  *other* side to move is where two latent differences hid, both now
  fixed to the C rule: `map.c` `generate_pawn_moves` only generates an
  en passant capture onto the capturing side's sixth (White) or third
  (Black) rank, and `zobrist.c` only hashes the en passant file when a
  pawn of the side to move stands on that same fixed rank beside it.
  pgnx's move generator and `Board.Zobrist` used to derive the
  capturing rank from the square itself, which let White's d2/f2 pawns
  "capture" onto e3 after `1. e4 --` (two extra moves in the
  `--evaluation` mobility term, a different `--hashcomments` value).
- **Every format writes `--`**, UCI included: `output.c` `print_move`
  writes `NULL_MOVE_STRING` in its SAN and long-algebraic cases alike
  and never `0000`. `chess.Move.UCI()` returns `0000` for the null move
  as the UCI protocol spells it, but the `uci` output format does not
  use it. `Z0`, the alternative spelling `lex.c` accepts, replays as a
  null move too and is written `--` in every format but Source, which
  keeps `Z0` as written (the C lexer rewrites it before parsing, a
  source-fidelity difference of the same kind as the others listed
  under "Parser data-model differences").
- **Without the flag** a main-line null move is an illegal move:
  `convert` rejects the game (Broken under `--keep-broken`, emitted in
  Source format as every broken game is) and `validate` reports it. The
  C tool replays the whole game before rejecting it; pgnx stops at the
  null move, which is only observable through `--keep-broken`'s partial
  replay (ECO classification of a broken game's plies). Null moves in
  variations are accepted by both tools without any flag (`grammar.c`
  `parse_move_unit` tests `RAV_level == 0`).
- **`--odds`** is `grammar.c` `check_for_odds`, run after the length
  bounds: a FEN tag whose middle four ranks are empty, White to move at
  move 1 (a fullmove field below 1 counts as 1, as `new_fen_board` reads
  it), and a difference between the sides in their number of pawns,
  knights, bishops, rooks or queens. `odds.pgn` reproduces the C tool's
  56 of 64 games.
- **Known divergence, not held to parity:** the stale en passant
  square can also be "captured" onto by the side that created it in
  the C tool: `decode.c`'s SAN path is looser than its move generator
  and accepts `1. e4 -- 2. dxe3` (emitting it, with a pawn on e3 and
  the e4 pawn still there) and `2... f5 3. -- exf6` (as `e7xf6ep`).
  pgnx resolves SAN against its generated legal moves and rejects both
  as illegal. Neither occurs in a real game record.
- **Known divergence, not held to parity:** a null move can leave a
  king en prise, and both tools then accept its capture (`1. Rh8+ --
  2. Rxa8`). pgnx appends a check glyph computed against the captured
  king's cached square (`Rxa8#`); the C tool writes `Rxa8` and warns
  about an inconsistent result. A position with no king is outside
  what either tool defines, so this is documented rather than matched.

## Ordered annotations (2026-11-03, beads `pgnx-ordered-annotations`)

`pgn.Move` now holds its NAGs, comments and variations as one ordered
list, `Annotations`, filled by the parser in source order; `NAGs()`,
`Comments()`, `LeadingComments()` and `Variations()` are views of it and
`AddComment` is how the rest of pgnx adds a comment. This is the data
model pgn-extract's grammar implies (`move_unit comment* (NAG+ comment*)*
(variant comment*)*`, with each NAG group's comments on the `Nag` and a
comment after `)` as the variation's `suffix_comment`), flattened: in
the C tool every one of those lists is printed in source order anyway.

- **Output order is source order.** `print_items_following_move` prints
  the move's `comment_list`, then each NAG with its comments, then the
  generated comments (`--evaluation`, `--fen-comments`,
  `--hash-comments`), then each variation with the comments that
  followed its closing paren.
  pgnx walks `Annotations` and emits the generated comments before the
  first variation (or after the last annotation when there is none),
  which is the same place. `--no-nags` keeps the comments that followed
  the NAGs and `--no-variations` keeps the comments that followed the
  variations, in position, as the C tool does. Oracle cases are pinned
  in `output/annotations_test.go`; `test-nagcomments` joined the golden
  harness (`TestGoldenConformanceNAGComments`, both the plain and the
  `-N -a` run) and `test-splitvariants.pgn` converts identically
  without the flag (and, since `--split-variants` was ported, with it:
  `TestGoldenConformance/test-splitvariants`), so both left
  `scripts/golden-difftest-known.txt`.
- **Generated match comments go before the NAGs.** `--mark-matches`
  (positional, material, piece-count, repetition and N-move-rule marks)
  reach the game through `append_comments_to_move`, which appends to
  `comment_list`, so the C tool emits `1. e4 { MATCH } $1 { a } (1. d4)`
  for `1. e4 $1 { a } (1. d4)`. `AddComment` inserts after the leading
  comments and before the first NAG or variation to match; before this
  change pgnx rendered the mark after the NAGs.
- **`--commented` counts only leading comments.** `comments_in_move_list`
  tests each move's `comment_list` and recurses into variations' moves;
  a comment that follows a NAG or a variation, and a variation's prefix
  comment, do not make a game commented. pgnx matched any move comment
  before; it now matches the C tool.
- **A result inside a variation is kept.** `parse_variant` accepts a
  result token before the closing paren and prints it after the
  variation's last move (`(1... c5 { x } 1-0)`), honouring
  `--noresults`. pgnx used to end the variation at the result, attach
  what followed to the move outside it and report an unmatched `)`.
  `pgn.Variation.Result` carries the token now, and `output` writes it;
  a result that ends a variation does not touch the game's own result
  or the missing-result rules. Two edges stay different, both outside
  the oracle corpus: a comment between that result and the `)` the C
  tool moves before the last move's NAGs (`append_comment` on
  `comment_list`), where pgnx keeps it in source order (`c5 $1 { x }
  1-0 { y }` is emitted as written, the C tool as `c5 { y } $1 { x }
  1-0`); and a move after the result the C tool reports as a missing
  `)` and reads as the next main-line move, where pgnx keeps it in the
  variation, before the result.
- **A NAG after a variation** stays a leniency; see "Parser data-model
  differences" above.
- **Corpus result.** `scripts/golden-difftest.sh`: 53 golden inputs
  identical, 1 known divergence (`nested-comment.pgn`), 0 unexpected.
  TWIC (`scripts/twicdiff.sh`, 2026-11-03): 744/749 identical, the same
  5 issues as the 2026-10-31 run, all the documented missing-result
  class.
