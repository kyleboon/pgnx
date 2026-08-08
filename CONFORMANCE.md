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

## Differential-testing findings (2026-08-07 run)

`scripts/difftest.sh` was run over every file in
`cmd/pgnx/testdata/golden/infiles/*.pgn` (56 files, default `san` format)
and over pgn-extract's own `eco.pgn` (2,014 games, ~12k lines) as a
larger real-world corpus. See `.superpowers/sdd/2026-08-07-pgnx-phase1/task-16-report.md`
for the full per-file table. Two findings were fixed; the rest are the
documented differences above.

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
