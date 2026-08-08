# Migrating from pgn-extract

pgnx does not accept pgn-extract's C flags directly — the CLI is redesigned
around subcommands (`convert`, `validate`, `info`) rather than one flat flag
namespace. This document maps the pgn-extract flags implemented in Phase 1
to their pgnx equivalents, and lists what is not yet ported.

## Flag mapping (Phase 1 scope)

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
  has an effect on `convert`, where it suppresses the "N games matched out
  of M." summary line. `validate` and `info` accept `--quiet` without
  error but it changes nothing about their output; use `validate
  --summary` for validate's equivalent output-collapsing option.
- pgn-extract's diagnostics (per-game errors, the run summary) go to
  stderr by default, or to a file with `-l`/`-L`. pgnx's equivalent is
  `--log FILE`; wording differs from the C tool (see CONFORMANCE.md).

## Not yet ported (Phase 2+)

The following pgn-extract flags have no pgnx equivalent yet. They require
game filtering/selection, deduplication, or ECO classification, which are
planned for later phases (see the design spec's phasing section):

- `-t` / `-T` — tag-file and single-criterion extraction (player, date,
  result, FEN, etc.)
- `-z` — material balance matching
- `-v` — textual variation matching
- `-x` — positional variation matching
- `-e` — ECO classification
- `-D` — suppress duplicate games
- `-d` — write duplicates to a file
- `-c` — check-file duplicate matching
- `-Z` — external hash table for large-dataset duplicate detection
- `-H` — match by Zobrist/Polyglot hash

Also not yet supported, regardless of flag:

- JSON output (`--json`)
- Argument files (`-A`/`argsfile`)
- `pgnx filter`, `pgnx dedupe`, `pgnx eco`, `pgnx split` — planned as new
  subcommands in later phases, not flags on `convert`

See [CONFORMANCE.md](CONFORMANCE.md) for behavioral differences in flags
that *are* implemented.
