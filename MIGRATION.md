# Migrating from pgn-extract

pgnx does not accept pgn-extract's C flags directly — the CLI is redesigned
around subcommands (`convert`, `filter`, `validate`, `info`) rather than one
flat flag namespace. This document maps the pgn-extract flags implemented
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
| `-f file_list` (list of input files) | pass the files as positional arguments instead |

Notes:

- `-W` output formats map to `--format`/`-f` values: `epd`→`epd`, `fen`→`fen`,
  `halg`→`halg`, `lalg`→`lalg`, `elalg`→`elalg`, `xlalg`→`xlalg`,
  `uci`→`uci`, `san`→`san` (the default). `-Wuci` also implicitly forces a
  very wide (5000-column) line width in the C tool; `pgnx convert -f uci`
  does the same unless `-w`/`--line-width` is given explicitly.
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
| `-Te ECO` | `pgnx filter --eco ECO` (prefix match on an existing ECO tag only; no classification) |
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

- `-Tf` (FEN criterion) and `FEN`/`FENPattern` lines in `-t` files are
  positional matching (Phase 3) and are rejected with an error.
- Malformed criteria files, a length bound of 0, and conflicting flags are
  usage errors (exit 1) rather than silently tolerated; see
  [CONFORMANCE.md](CONFORMANCE.md#filtering-phase-2-pgnx-filter).
- A run in which no game matches exits 0; "nothing matched" is not an
  error. Broken games and games rejected by `--no-bad-results` make the
  exit status 2.

## Not yet ported (Phase 3+)

The following pgn-extract flags have no pgnx equivalent yet:

- Positional matching (Phase 3): `-Tf`, `FEN`/`FENPattern` lines in `-t`
  files, `-x`, `-v`, `-H`, `-P`, `--startply`, `--matchplylimit`,
  `--addlabeltag`, `--wtm`/`--btm`, `--vanywhere`
- `-e` / `-E` — ECO classification (Phase 3)
- Duplicate detection (Phase 4): `-D`, `-U`, `-d`, `-c`, `-Z`,
  `--fuzzydepth`, `--deletesamesetup`
- `--odds` — needs null-move support (`--allownullmoves`), which `replay`
  does not have
- Output-side transforms (a planned "convert transforms" mini-phase):
  `--plycount`, `--totalplycount`, `--addhashcode`, `--dropply`,
  `--plylimit`, `--linenumbers`, `--splitvariants`, `--fencomments`,
  `--evaluation`, `--hashcomments`, `-F`, `--commentlines`, `--notags`,
  `--nochecks`, `--addelotags`, `--addfideidtags`, `--dropbefore`

Also not yet supported, regardless of flag:

- JSON output (`--json`)
- Argument files (`-A`/`argsfile`)
- `-#` file splitting
- `pgnx dedupe`, `pgnx eco`, `pgnx split` — planned as new subcommands in
  later phases, not flags on `convert`

See [CONFORMANCE.md](CONFORMANCE.md) for behavioral differences in flags
that *are* implemented.
