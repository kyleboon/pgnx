# pgnx

pgnx is a Go port of [pgn-extract](https://www.cs.kent.ac.uk/people/staff/djb/pgn-extract/),
David J. Barnes' C tool for searching, filtering, and reformatting chess
games recorded in [PGN](https://en.wikipedia.org/wiki/Portable_Game_Notation).
It provides both a reusable Go library (`chess`, `pgn`, `replay`, `filter`,
`eco`, `output`) and a CLI (`pgnx`) that replays every game on an internal board
with full legality checking, the same semantic depth as the original.

pgnx is licensed under [GPL-3.0](LICENSE), the same license as pgn-extract.

This is a from-scratch re-architecture, not a transliteration: the CLI is
redesigned around subcommands rather than pgn-extract's large flag surface.
See [MIGRATION.md](MIGRATION.md) for how old flags map to new invocations,
and [CONFORMANCE.md](CONFORMANCE.md) for documented behavioral differences
from the C tool.

**Status:** Phase 3 — core parsing, move replay/validation, output
formatting (`convert`, `validate`, `info`), game selection (`filter`: tag
criteria, length, end state, material balance, result consistency,
positions reached, FEN patterns, move sequences) and ECO classification
(`--eco-file`, `--eco-split`). Duplicate detection and output-side
transforms are planned for later phases; see
[docs/superpowers/specs](docs/superpowers/specs) for the design specs and
phasing plan.

## Install

Homebrew (macOS and Linux):

```sh
brew install kyleboon/tap/pgnx
```

Prebuilt binaries for Linux, macOS and Windows (amd64 and arm64) are on the
[releases page](https://github.com/kyleboon/pgnx/releases), with a
`checksums.txt` to verify them against. The binaries are not code-signed:
Windows SmartScreen warns on first run, and on macOS the Homebrew cask clears
the quarantine attribute for you; a manually downloaded binary may need
`xattr -d com.apple.quarantine pgnx`.

With a Go toolchain:

```sh
go install github.com/kyleboon/pgnx/cmd/pgnx@latest
```

`pgnx --version` reports the version, commit and build date.

## CLI

pgnx has four subcommands. Every one reads PGN from files given as
arguments, or from stdin when none are given, and writes to stdout unless
redirected.

### `pgnx convert`

Parses, validates (via replay), and re-emits games — the workhorse
subcommand. Use `-f`/`--format` to change the move notation, and flags like
`--no-comments`, `--no-variations`, or `--seven-tag-roster` to strip
elements from the output.

```sh
pgnx convert -f uci -o out.pgn games.pgn

# Add ECO/Opening/Variation tags from an ECO file, splitting the output
# into A.pgn .. E.pgn (and noeco.pgn) by the first letter of the code
pgnx convert --eco-file eco.pgn --eco-split 1 games.pgn
```

### `pgnx filter`

Selects games. It takes every `convert` output flag plus selection
criteria, so `filter` with no criteria is `convert`. Tag criteria use
pgn-extract's `-t` grammar (`Tag [op] "value"`, with `<`, `<=`, `<>`,
`>`, `>=`, `=`, `=~`); criteria on the same tag are alternatives and
criteria on different tags must all hold.

```sh
# Fischer's games from 1970, by name prefix and date prefix
pgnx filter --player Fischer --date 1970 games.pgn

# Games of at least 45 moves between players rated 2500+
pgnx filter --min-moves 45 --tag 'WhiteElo >= "2500"' --tag 'BlackElo >= "2500"' games.pgn

# Games containing a threefold repetition, with each repeated position marked
pgnx filter --repetition --mark-matches REPEAT games.pgn

# Route matched and unmatched games to separate files
pgnx filter --checkmate -o mates.pgn --unmatched others.pgn games.pgn

# Games reaching the Najdorf position, by any move order, with the
# matching move marked
pgnx filter --position 'e4 c5 Nf3 d6 d4 cxd4 Nxd4 Nf6 Nc3 a6' --mark-matches MATCH games.pgn

# Games with a white knight on e4 beside a black pawn on d4, supported by
# a pawn on d3, or the mirror image for Black; matched games get a
# MatchLabel tag (I for the mirrored form)
pgnx filter --fen-pattern-i '*/*/*/*/???pN???/???P????/*/*' --add-label-tag games.pgn

# Games opening 1. c4 Nf6 2. Nc3 e6 3. d4 Bb4 in exactly that order
pgnx filter --exact-order --sequence 'c4 Nf6 Nc3 e6 d4 Bb4' games.pgn

# Classify by ECO, then keep only the B9x (Najdorf) games
pgnx filter --eco-file eco.pgn --eco B9 games.pgn
```

### `pgnx validate`

Checks games for syntax problems and illegal moves, without writing any PGN
output. Findings go to stdout; `--summary` collapses them to a single tally
line.

```sh
pgnx validate --summary games.pgn
```

### `pgnx info`

Reports aggregate statistics — game count, ply counts, and result
breakdown — over a collection of games.

```sh
pgnx info games.pgn
```

## Library quick-start

The library is organized in layers: `pgn` parses PGN text into a
`*pgn.Game` (preserving source fidelity — SAN as written, comments, NAGs,
variations); `replay` applies a game's moves to a `chess.Board`, validating
legality and resolving the detail (FEN, checks, captures) that non-Source
output formats need; `filter` evaluates selection criteria over a game and
its replay; `eco` classifies a replayed game against an ECO file; `output`
renders a game, plus its replay result, back out as text.

```go
package main

import (
	"os"

	"github.com/kyleboon/pgnx/output"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

func main() {
	f, err := os.Open("games.pgn")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	scanner := pgn.NewScanner(f, pgn.ParserOptions{})
	w := output.NewWriter(os.Stdout, output.Options{
		Format:    output.SAN,
		LineWidth: 75,
	})

	for scanner.Next() {
		game := scanner.Game() // *pgn.Game
		result, err := replay.Game(game, replay.Options{})
		if err != nil {
			continue // illegal move; skip this game
		}
		if err := w.WriteGame(game, result); err != nil {
			panic(err)
		}
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
}
```

To select games, build a `filter.Options` and call `filter.Evaluate(game,
result, &opts)` between replay and output; `filter.ParseCriterion` and
`filter.TagMatcher` give the same tag-criteria semantics as the CLI, and
`filter.PositionCriteria` / `filter.ParseSequence` the positional and
move-sequence ones. To classify, `eco.Load` an ECO file once, then
`table.Classify(result)` and `eco.Apply(game, entry)` before output.

`pgn.Scanner` streams one game at a time (`bufio.Scanner`-style), so
million-game files process in constant memory. `chess` has no PGN
knowledge and is independently usable: `chess.NewBoard()`, `chess.FromFEN`,
move generation, legality, and Zobrist hashing.

## Documentation

- [docs/superpowers/specs](docs/superpowers/specs) — design spec and
  architecture
- [MIGRATION.md](MIGRATION.md) — mapping from pgn-extract's C flags to
  pgnx's subcommands and flags
- [CONFORMANCE.md](CONFORMANCE.md) — documented intentional differences
  from the C tool, and known gaps

## Developing and releasing

[mise](https://mise.jdx.dev) pins the Go and GoReleaser versions and defines
the tasks CI runs, so local and CI runs are identical:

```sh
mise install          # pinned Go + GoReleaser
mise run ci           # build, vet, gofmt, go test -race, goreleaser check
mise run snapshot     # full release dry-run into dist/ (no tag, no token)
```

CI runs `mise run ci` on Linux, macOS and Windows for every push to `main`
and every pull request. Releasing is a human decision: push a semver tag and
the release workflow builds every target, publishes a GitHub Release with
`checksums.txt`, and updates the Homebrew tap.

```sh
git tag -a v0.2.0 -m "pgnx 0.2.0"
git push origin v0.2.0
```

A tag such as `v0.2.0-rc1` becomes a prerelease and leaves the Homebrew cask
untouched. Tags not on `main` are refused. See
[docs/adr/0001](docs/adr/0001-release-via-tag-push-and-homebrew-tap.md) for
the reasoning and `scripts/release-setup-wizard.sh` for the one-time setup
(tap repo and token).

## Credit

pgnx ports the design and semantics of
[pgn-extract](https://www.cs.kent.ac.uk/people/staff/djb/pgn-extract/) by
**David J. Barnes** (d.j.barnes@kent.ac.uk). The C implementation remains
the behavioral reference this project is tested against.
