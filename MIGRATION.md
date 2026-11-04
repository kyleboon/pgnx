# Migrating from pgn-extract

pgnx does not accept pgn-extract's C flags directly — the CLI is redesigned
around subcommands (`convert`, `filter`, `split`, `validate`, `info`) rather
than one flat flag namespace. This document maps the pgn-extract flags implemented
so far to their pgnx equivalents, and lists what is not yet ported.

## Output flags (Phase 1 scope; also accepted by `filter`)

| pgn-extract | pgnx |
|---|---|
| `-7` | `pgnx convert --seven-tag-roster` |
| `-C` | `pgnx convert --no-comments` |
| `-N` | `pgnx convert --no-nags` |
| `-V` | `pgnx convert --no-variations` |
| `-w N` | `pgnx convert --line-width N` (or `-w N`) |
| `-W<fmt>` | `pgnx convert -f <fmt>` (e.g. `-Wuci` → `convert -f uci`) |
| `-Wsan<letters>`, `-Welalg<letters>`, `-Wxlalg<letters>` | `pgnx convert -f <fmt> --piece-letters <letters>` (e.g. `-WsanBSLTDK` → `convert --piece-letters BSLTDK`; also on `filter`, `split`) |
| `-o F` | `pgnx convert -o F` |
| `-a F` | `pgnx convert -o F --append` |
| `-r` | `pgnx validate` |
| `-s` / `--quiet` | `pgnx convert --quiet` |
| `-l F` | `pgnx convert --log F` (or `validate`/`info --log F`) |
| `-R tagorder` | `pgnx convert --roster TAG,TAG,...` (explicit comma-separated list instead of a file) |
| `--nestedcomments` | `pgnx convert --nested-comments` (also on `validate`, `info`) |
| `--nomovenumbers` | `pgnx convert --no-move-numbers` |
| `--noresults` | `pgnx convert --no-results` |
| `--keepbroken` | `pgnx convert --keep-broken` |
| `--allownullmoves` | `pgnx convert --allow-null-moves` (also on `filter`, `split`, `validate`; null moves in variations never need it) |
| `--json` | `pgnx convert --json` (also on `filter`, `split`); `-f` still picks the move spelling, and `epd`, `fen`, `cm` and `--eco-split` are usage errors where the C tool warns and drops `--json` |
| `-f file_list` (list of input files) | pass the files as positional arguments instead (or list them in an argument file, below) |
| `-A FILE` (argument file) | `pgnx <command> @FILE`, in pgnx's own grammar (see [Argument files](#argument-files)) |

Notes:

- `-W` output formats map to `--format`/`-f` values: `epd`→`epd`, `fen`→`fen`,
  `halg`→`halg`, `lalg`→`lalg`, `elalg`→`elalg`, `xlalg`→`xlalg`,
  `uci`→`uci`, `cm`→`cm` (ChessMaster), `san`→`san` (the default). `-Wuci` also implicitly forces a
  very wide (5000-column) line width in the C tool; `pgnx convert -f uci`
  does the same unless `-w`/`--line-width` is given explicitly.
- `--piece-letters` takes the six letters pieces are spelled with, in the
  order pawn, knight, bishop, rook, queen, king (`PNBRQK` is the default);
  `X+Y` spells a piece as the two characters `XY`. It applies to `san`,
  `elalg` and `xlalg` output and is a usage error with any other format,
  where the C tool ignores the suffix (see CONFORMANCE.md).
- pgn-extract's `-r` only reports errors; it does not extract PGN output.
  `pgnx validate` is the direct equivalent — findings go to stdout, with
  `--summary` to collapse them to a single tally line.
- `--quiet` is accepted as a persistent flag on every subcommand but only
  has an effect on `convert` and `filter`, where it suppresses the "N games
  matched out of M." summary line. `validate` and `info` accept `--quiet` without
  error but it changes nothing about their output; use `validate
  --summary` for validate's equivalent output-collapsing option.
- pgn-extract's diagnostics (per-game errors, the run summary) go to
  stderr by default, or to a file with `-l`/`-L`. pgnx's equivalent is
  `--log FILE`; wording differs from the C tool (see CONFORMANCE.md).

## Selection flags (Phase 2 scope: `pgnx filter`)

`pgnx filter` accepts every `convert` output flag plus the criteria below;
with no criteria it behaves exactly like `convert`. Criteria on the same
tag are alternatives (ORed); criteria on different tags must all hold.

| pgn-extract | pgnx |
|---|---|
| `-t FILE` | `pgnx filter --tag-file FILE` (same line grammar: `Tag [op] "value"`) |
| `-Tp NAME` | `pgnx filter --player NAME` |
| `-Tw NAME` | `pgnx filter --white NAME` |
| `-Tb NAME` | `pgnx filter --black NAME` |
| `-Td DATE` | `pgnx filter --date DATE` (`b1962`/`a1962` prefixes work as in C) |
| `-Tr RESULT` | `pgnx filter --result RESULT` |
| `-Te ECO` | `pgnx filter --eco ECO` (prefix match on the ECO tag; with `--eco-file` it tests the classified tag, as in C) |
| `-Ta NAME`, `-Tt TC`, any other single criterion | `pgnx filter --tag 'Annotator "NAME"'`, `--tag 'TimeControl >= "180"'` — the `-t` line grammar on the command line, repeatable |
| `--tagsubstr` | `pgnx filter --tag-substring` |
| `-S` | `pgnx filter --soundex` |
| `--higherratedwinner` / `--lowerratedwinner` | `pgnx filter --higher-rated-winner` / `--lower-rated-winner` |
| `--nosetuptags` / `--onlysetuptags` | `pgnx filter --no-setup-tags` / `--only-setup-tags` |
| `-bl N` / `-bu N` / `-b N` (`-be N`) | `pgnx filter --min-moves N` / `--max-moves N` / `--moves N` |
| `-pl N` / `-pu N` / `-p N` (`-pe N`) | `pgnx filter --min-ply N` / `--max-ply N` / `--ply N` |
| `--checkmate` / `--stalemate` / `--insufficient` | same names |
| `--underpromotion` / `--commented` | same names |
| `--fifty` / `--seventyfive` | `pgnx filter --fifty` / `--seventy-five` |
| `--odds` | `pgnx filter --odds` (pair with `--allow-null-moves` as the C test does; odds games often record the given move as `--`) |
| `--repetition` / `--repetition5` | `pgnx filter --repetition` / `--repetition=5` |
| `--piececount N` | `pgnx filter --piece-count N` |
| `-z FILE` / `--materialz SPEC` | `pgnx filter --material-file FILE` / `--material SPEC` |
| `-y FILE` / `--materialy SPEC` | `pgnx filter --white-material-file FILE` / `--white-material SPEC` |
| `--markmatches TEXT` | `pgnx filter --mark-matches TEXT` (`FEN` inserts the position, as in C) |
| `--addmatchtag` | `pgnx filter --add-match-tag` |
| `-n FILE` | `pgnx filter --unmatched FILE` (`-` for stdout) |
| `--suppressmatched` | `pgnx filter --suppress-matched` |
| `--selectonly LIST` / `--skipmatching LIST` | `pgnx filter --select-only LIST` / `--skip-matching LIST` |
| `--stopafter N` / `--firstgame N` / `--gamelimit N` | `pgnx filter --stop-after N` / `--first-game N` / `--game-limit N` |
| `--nobadresults` / `--fixresulttags` | `pgnx filter --no-bad-results` / `--fix-result-tags` |

Notes:

- `-Tf` and `FEN`/`FENPattern`/`FENPatternI` lines in `-t` files are
  positional criteria; see the next section.
- Malformed criteria files, a length bound of 0, and conflicting flags are
  usage errors (exit 1) rather than silently tolerated; see
  [CONFORMANCE.md](CONFORMANCE.md#filtering-phase-2-pgnx-filter).
- A run in which no game matches exits 0; "nothing matched" is not an
  error. Broken games and games rejected by `--no-bad-results` make the
  exit status 2.

## Positional, sequence and ECO flags (Phase 3 scope)

Positional and move-sequence criteria are `pgnx filter` flags. ECO
classification and ECO-split output are accepted by both `convert` and
`filter`.

| pgn-extract | pgnx |
|---|---|
| `-x FILE` | `pgnx filter --position-file FILE` (one move line per line, as in C); one line can be given inline with `--position 'e4 c5 Nf3'` |
| `FEN "..."` line in a `-t` file | accepted by `pgnx filter --tag-file FILE` and `--tag 'FEN "..."'`; `--fen '...'` gives one inline |
| `-Tf PATTERN` / `--fenpattern PATTERN` | `pgnx filter --fen-pattern PATTERN` |
| `--fenpatterni PATTERN` | `pgnx filter --fen-pattern-i PATTERN` |
| `FENPattern "..." [label]` / `FENPatternI "..." [label]` lines in a `-t` file | accepted by `--tag-file FILE` and `--tag '...'` with the same grammar |
| `-H HASH` | `pgnx filter --hash HASH` |
| `--startply N` | `pgnx filter --start-ply N` |
| `--matchplylimit N` | `pgnx filter --match-ply-limit N` |
| `--wtm` / `--btm` | `pgnx filter --white-to-move` / `--black-to-move` |
| `--addlabeltag` | `pgnx filter --add-label-tag` |
| `--markmatches TEXT` | `pgnx filter --mark-matches TEXT` also marks positional matches, as in C |
| `-v FILE` | `pgnx filter --sequence-file FILE`; one line inline with `--sequence 'e4 c5 * d6'` |
| `-P` | `pgnx filter --exact-order` |
| `--vanywhere` | `pgnx filter --sequence-anywhere` |
| `-e FILE` | `pgnx convert --eco-file FILE` / `pgnx filter --eco-file FILE` |
| `-e` | `pgnx convert --eco-default` / `pgnx filter --eco-default` (`$ECO_FILE`, else `./eco.pgn`) |
| `-E N` | `pgnx convert --eco-split N` / `pgnx filter --eco-split N` |
| `-e FILE -TeB1` | `pgnx filter --eco-file FILE --eco B1` (the criterion sees the classified tag) |

Notes:

- `--eco-file` always needs a path; `--eco-default` is the C tool's `-e`
  with no file name, reading `$ECO_FILE` or, when that is unset or
  empty, `eco.pgn` in the current directory. The two flags are mutually
  exclusive. A copy of pgn-extract's `eco.pgn` is in the repository at
  `cmd/pgnx/testdata/golden/eco.pgn`.
- `--eco-split N` writes to `<prefix>.pgn` (or `noeco.pgn`) in the
  current directory, appending, exactly as `-E` does, and cannot be
  combined with `--output`.
- Malformed patterns, hashes, move lines and sequences are usage errors
  (exit 1) rather than logged and skipped; see
  [CONFORMANCE.md](CONFORMANCE.md#positional-matching-and-eco-phase-3).

## Output transforms (Phase 2b scope; `convert` flags, so also `filter`'s)

Options that rewrite or annotate a game on output rather than select it.
They are `convert` flags, so `filter` and `split` take them too.

| pgn-extract | pgnx |
|---|---|
| `--plycount` | `pgnx convert --ply-count` |
| `--totalplycount` | `pgnx convert --total-ply-count` |
| `--addhashcode` | `pgnx convert --add-hash-code` |
| `--addelotags FILE` | `pgnx convert --add-elo-tags FILE` (same tab-separated name/rating file) |
| `--addfideidtags FILE` | `pgnx convert --add-fide-id-tags FILE` |
| `--dropply N` | `pgnx convert --drop-ply N` (negative N keeps the last -N plies, as in C) |
| `--dropbefore TEXT` | `pgnx convert --drop-before TEXT` |
| `--plylimit N` | `pgnx convert --ply-limit N` |
| `--linenumbers MARKER` | `pgnx convert --line-numbers MARKER` |
| `--splitvariants` | `pgnx convert --split-variants` |
| `--splitvariants N` | `pgnx convert --split-variants --split-depth N` |
| `--fencomments` | `pgnx convert --fen-comments` |
| `--hashcomments` | `pgnx convert --hash-comments` |
| `--evaluation` | `pgnx convert --evaluation` |
| `-F` | `pgnx convert --final-fen` |
| `--commentlines` | `pgnx convert --comment-lines` |
| `--notags` | `pgnx convert --no-tags` |
| `--nochecks` | `pgnx convert --no-checks` |
| `-#N` | `pgnx split --games N` |
| `-#N,M` | `pgnx split --games N --start M` |

Notes:

- `pgnx split` is `convert` writing numbered files (`1.pgn`, `2.pgn`, ...;
  `.epd`/`.fen` by format) of `--games` games each into the current
  directory, exactly as `-#` does. It takes every `convert` flag except
  `--output`/`--append` and `--eco-split`, which are usage errors (the C
  tool warns and ignores `-o`). To split a selection, pipe `pgnx filter`
  into `pgnx split`.
- `--drop-ply` and `--drop-before` run after selection and classification,
  as the C tool's rewrite does, so criteria and `--eco-file` see the
  game as parsed. A game with fewer plies than asked is reported and
  emitted nowhere (exit 2); the C tool drops it silently.
- `--split-variants` emits each game as its main line followed by one
  game per variation, each stripped of variations, as `--splitvariants`
  does; `--split-depth N` is the C flag's optional depth (0, the
  default, is unlimited). It conflicts with `--no-variations` (a usage
  error, as `-V` is in the C tool) and does nothing under `--format uci`,
  which drops variations. The other transforms then run over each
  emitted game on its own (the C tool compounds them across the split;
  see CONFORMANCE.md).
- `-Fpattern` (replacing a matching comment with a FEN), `--fencommentformat`
  and `--quiescent` are not ported; see CONFORMANCE.md.
- `--no-tags` with `--seven-tag-roster` or `--roster`, and a negative
  `--ply-limit`, are usage errors (exit 1), as the C tool exits 1 for them.

## Argument files

pgn-extract's `-A FILE` reads a file in its own grammar (`argsfile.c`): a
`:-X` line per flag, with `:-t`, `:-v`, `:-x`, `:-z`, `:-y` and `:-R` lines
switching the following bare lines into criteria or roster lines, and
`:file.pgn` naming an input. pgnx does not read that grammar; it has its own
argument files, written `@FILE` anywhere on the command line (before or after
the subcommand), which cobra then parses exactly as if the file's arguments
had been typed there:

- one argument per line, except that a line starting with `-` is a flag
  followed by its value, split at the first run of white space
  (`--tag White "Fischer"` is `--tag` and `White "Fischer"`; `-o out.pgn` is
  `-o` and `out.pgn`; `--line-width=60` is one argument);
- a line not starting with `-` is one positional argument, white space and
  all: the subcommand, an input file, or a nested `@FILE`;
- blank lines and comment lines, whose first non-blank character is `%`,
  are ignored (the `--tag-file`, `--position-file` and `--sequence-file`
  files take `%` as a comment only in the first column, as pgn-extract does);
- paths in a file, nested `@FILE` lines included, are relative to the
  working directory, as pgn-extract's are;
- `--` ends expansion: everything after it is passed through verbatim.

To translate a pgn-extract argsfile: drop the `:` prefix and map each flag
with the tables above; turn a `:-t` section into one `--tag` line per
criterion (or point `--tag-file` at a file holding just those lines), and a
`:-x`/`:-v`/`:-z`/`:-y` section into `--position`/`--sequence`/`--material`
lines or their `-file` flags; `:file.pgn` lines become bare `file.pgn`
lines. The C test `test-FENPattern.txt`

```
:-t
% Find a supported Knight on e4.
FENPattern "*/*/*/*/???pN???/???P????/*/*"
% Find a supported Knight for White on d4
% or Black on d5.
FENPatternI "*/*/*/*/???Np???/????P???/*/*" Nd4th
```

becomes

```
% Find a supported Knight on e4.
--tag FENPattern "*/*/*/*/???pN???/???P????/*/*"
% Find a supported Knight for White on d4
% or Black on d5.
--tag FENPatternI "*/*/*/*/???Np???/????P???/*/*" Nd4th
```

and `pgn-extract -Atest-FENPattern.txt --addlabeltag --markmatches HERE
petrosian.pgn` becomes `pgnx filter @test-FENPattern.txt --add-label-tag
--mark-matches HERE petrosian.pgn` (`cmd/pgnx/testdata/argfiles/`, golden
row `test-FENPattern-argfile`).

## Not yet ported (Phase 4+)

The following pgn-extract flags have no pgnx equivalent yet:

- Duplicate detection (Phase 4): `-D`, `-U`, `-d`, `-c`, `-Z`,
  `--fuzzydepth`, `--deletesamesetup`
- `-Fpattern`, `--fencommentformat`, `--quiescent` — the comment-replacing
  form of `-F`, its format string, and the quiescence extension of
  `--plylimit`

Also not yet supported, regardless of flag:

- `pgnx dedupe` — planned as a new subcommand in a later phase, not a
  flag on `convert`

See [CONFORMANCE.md](CONFORMANCE.md) for behavioral differences in flags
that *are* implemented.
