# pgnx: Documentation site on GitHub Pages

**Date:** 2026-11-02
**Status:** Proposed
**Module:** `github.com/kyleboon/pgnx`
**Builds on:** Phases 1, 2, 2b and 3 (complete); documents the shipped feature set only.

## Problem Statement

pgnx has a capable CLI (five subcommands, 34 to 93 flags each, several
mini-languages for criteria and patterns) and a reusable Go library, but the
only way to learn it is a 230-line README, `--help` output, and three
contributor documents written in terms of the C tool (MIGRATION.md,
CONFORMANCE.md, the design specs). A chess player with a pile of PGN files
cannot find out what pgnx does for them, how to get started, or what a flag's
grammar is, without reading source or already knowing pgn-extract. There is
no published site, and nothing checks that the examples that do exist still
produce the output they show.

## Solution

A documentation site at `kyleboon.github.io/pgnx`, built with MkDocs Material
from Markdown under `docs/`, deployed by GitHub Actions on every push to
`main`. It is organised by Diátaxis: a Getting Started tutorial that runs real
commands against a sample file, ten task-shaped guides, a reference section
whose per-command pages are generated from the CLI itself so they cannot
drift, hand-written reference pages for each grammar and file format, and an
explanation section covering how pgnx processes a game and its relationship to
pgn-extract. The site speaks in pgnx's own terms, assumes the reader knows
chess notation, gives generous credit to pgn-extract, and never mentions
internal phase numbers. The tutorial's shown output is verified in CI, the
generated reference is checked for drift in CI, and the site build is strict,
so the docs stay as reliable as the conformance suite keeps the output.

## User Stories

1. As a chess player with a PGN file, I want a landing page that tells me in one screen what pgnx does, so that I can decide in a minute whether it solves my problem.
2. As a chess player, I want install instructions for Homebrew, prebuilt binaries and `go install`, so that I can get a working `pgnx` on my machine without reading the README.
3. As a new user, I want a tutorial I can follow by copy-pasting, with a downloadable sample file, so that every command I run produces exactly the output the page shows.
4. As a new user, I want the tutorial to walk through `info`, `validate`, `convert`, `filter` and `split` in that order, so that I meet each subcommand once with a concrete purpose.
5. As a user, I want a guide for finding my games against a player or within a date range, so that I can pull my own games out of a database export.
6. As a user, I want a guide for keeping only games in an opening, by ECO code or by position reached, so that I can build an opening repertoire file.
7. As a user, I want a guide for finding games that reach a position or play a move sequence, so that I can study a specific line.
8. As a user, I want a guide for stripping comments, variations and NAGs, so that I can produce a clean file for an engine or a database import.
9. As a user, I want a guide for converting games to the uci or long-algebraic notations, so that I can feed them to a chess engine.
10. As a user, I want a guide for splitting a huge file into numbered chunks, so that I can handle a multi-gigabyte database in pieces.
11. As a user, I want a guide for finding illegal or broken games, so that I can clean a corpus before relying on it.
12. As a user, I want a guide for adding ECO, PlyCount, Elo and FIDE ID tags, so that I can enrich a file with data it lacks.
13. As a user, I want a guide for trimming opening moves and emitting games from a FEN start, so that I can produce middlegame-only studies.
14. As a user, I want a guide for running pgnx in a shell pipeline over multi-gigabyte input, so that I know it streams and how to combine it with other tools.
15. As a user reading a flag's help, I want a reference page per subcommand listing every flag with its description, so that I can look up a flag without running `--help`.
16. As a user, I want the per-command reference to be generated from the CLI, so that the published flags are never out of date with the installed binary.
17. As a user, I want a reference page for the tag-criteria grammar, so that I know which operators exist and how criteria on the same and different tags combine.
18. As a user, I want a reference page for material-balance patterns, so that I can express "two bishops against bishop and knight".
19. As a user, I want a reference page for FEN patterns with labels and material constraints, so that I can search by piece configuration.
20. As a user, I want a reference page for move sequences and position lines, so that I understand exact-order versus any-order matching and where in the game a match may occur.
21. As a user, I want a reference page for every output format (san, halg, lalg, elalg, xlalg, uci, epd, Source), so that I know what each emits and when Source is the only option.
22. As a user, I want a reference page for the ECO file format and `--eco-split`, so that I can supply my own opening classification.
23. As a user, I want a reference page for the player-info file used by the Elo and FIDE ID tag options, so that I can prepare that file correctly.
24. As a user scripting pgnx, I want a reference page for exit codes and diagnostics, so that my scripts can tell success from bad input.
25. As a user, I want an explanation of how pgnx processes a game (normalisation, replay, broken games, classification, criteria, transforms, output), so that I can predict what a combination of flags will do.
26. As a user, I want an explanation of what counts as a failure and how it is reported, so that I understand a non-zero exit on a file I thought was fine.
27. As a user, I want an explanation of streaming and memory behaviour, so that I can trust pgnx on inputs larger than RAM.
28. As a pgn-extract user, I want a page explaining pgnx's relationship to pgn-extract, so that I know what was kept, what changed and what is not ported yet.
29. As a pgn-extract user, I want each flag's help to name the pgn-extract flag it corresponds to, so that I can translate a command I already know.
30. As a reader, I want a Credits and licence page naming David J. Barnes and pgn-extract, the GPL-3.0 licence, and the origin of the sample file, so that credit is given where it is due.
31. As a Go developer, I want a page showing how to use the library (scan, replay, filter, classify, write) and a link to pkg.go.dev, so that I can embed pgnx without reading the CLI source.
32. As a reader, I want a short glossary of pgnx-specific terms (criterion, output option, transform, broken game, matched game, Source format), so that the words used across the site are defined once.
33. As a reader on a phone, I want the site to be readable at narrow widths with working search and a light/dark toggle, so that I can consult it anywhere.
34. As a reader, I want no analytics or cookies, so that reading the docs costs me nothing in privacy.
35. As a maintainer, I want `mise run docs-serve` to preview the site locally with no tools installed beyond mise, so that writing docs is as reproducible as building the code.
36. As a maintainer, I want `mise run docs-build` to fail on a broken link or a page missing from the navigation, so that the site is never published broken.
37. As a maintainer, I want `mise run docs-gen` to regenerate the per-command reference and `mise run ci` to fail if the committed pages differ, so that the reference cannot drift from the CLI.
38. As a maintainer, I want `mise run docs-check` to run the tutorial's commands against the sample and diff the shown output, so that the tutorial cannot silently go stale.
39. As a maintainer, I want the docs build and docs-check to run as one extra ubuntu-only CI job, so that the three-OS `mise run ci` matrix stays Go-only and fast.
40. As a maintainer, I want the site deployed by a SHA-pinned GitHub Actions workflow on pushes to `main`, so that publishing follows the same pinning convention as CI and release.
41. As a maintainer, I want the release binaries unchanged in size and dependencies, so that generating docs never costs users anything.
42. As a maintainer, I want the README reduced to a front door (description, install, two examples, site link, licence, credit), so that there is one place for everything else to go stale.
43. As a maintainer, I want MIGRATION.md and CONFORMANCE.md left in place and off the site until the last C feature lands, so that contributors keep their oracle notes and the site stays about pgnx.
44. As a maintainer, I want the work tracked as one epic with one child per deliverable, so that it can be picked up piecemeal by agents.
45. As a maintainer, I want the site unversioned and tracking `main`, with features newer than the latest release marked, so that there is no versioning machinery to maintain before 1.0.

## Implementation Decisions

### Site generator and layout

- MkDocs with the Material theme. `mkdocs.yml` lives at the repository root; `docs/` is the content root. The existing developer directories under `docs/` (`adr/`, `agents/`, `superpowers/`) are excluded from the build with MkDocs' exclusion setting and never publish.
- Navigation, in order: Home, Getting Started (install, tutorial), Guides (ten pages), Reference (CLI: one page per subcommand; grammars and formats: eight pages), Explanation (processing model, failures and exit codes, streaming and memory, pgnx and pgn-extract), Library, Glossary, Credits and licence.
- Theme: Material defaults with one primary colour, a chess-piece logo and favicon, light/dark toggle, built-in search, repository link in the header. No analytics, no custom CSS beyond the palette.
- Site URL is the GitHub Pages default for the repository. No custom domain. Unversioned; a "Since vX.Y" admonition marks features added after the latest release, and the landing page lists capabilities and a short "not yet" line (duplicate detection, engine analysis) without phase numbers.

### Voice and content rules

- Written for chess players and data wranglers first, pgn-extract users second, Go developers third. Assumes PGN, SAN, FEN and ECO are known; links out to the standards.
- Vocabulary follows GLOSSARY.md: a *criterion* is one condition `filter` selects by; a *transform* rewrites a game on the way out; an *output option* changes only rendering; a *broken game* is one whose moves did not fully replay and is kept; *Source format* is the only format a broken game can be emitted in; *matched* and *unmatched* describe criteria outcome.
- pgn-extract is credited generously and specifically: the landing page carries a two-sentence lineage paragraph; the Explanation section has a full "pgnx and pgn-extract" page (who David J. Barnes is, what pgn-extract is, that pgnx is a from-scratch re-architecture around subcommands, a streaming scanner and a reusable library rather than a transliteration, that most features are kept, that the C tool is the oracle pgnx is tested against for byte-identical output, the deliberate differences in exit codes and diagnostics, what is not yet ported); a Credits and licence page covers GPL-3.0 and the sample file's origin. Feature pages describe pgnx on its own terms and do not explain features as "the same as C flag X".
- The pgn-extract alias in parentheses at the end of each flag's help text is kept, and explained once on the lineage page.

### Generated CLI reference

- A hidden `gen-docs` subcommand is registered only when the binary is built with the `docsgen` build tag. It uses cobra's documentation package to write one Markdown page per subcommand into the reference directory. Release binaries are built without the tag and gain no dependency.
- Subcommand `Long` and `Example` texts are the published prose for those pages, so they are expanded where thin (convert, validate, info, split) to a paragraph of purpose plus two or three examples each.
- `mise run docs-gen` runs the generator; `mise run ci` gains a `docs-gen-check` task that regenerates and fails on a non-empty diff. Generated pages are committed.

### Tutorial and sample

- The sample is the 34-game Fischer collection already vendored from the C test corpus, copied to the docs assets directory and served by the site at a stable URL.
- The tutorial runs, in order, `info`, `validate`, `convert`, `filter`, `split` against it and shows real output for each.
- `mise run docs-check` extracts each command block from the tutorial that is paired with an output block, runs it with the freshly built binary against the sample, and diffs. The marker format for pairing a command with its expected output is chosen during implementation and documented in the tutorial source.

### Toolchain and CI

- `uv` is added to the mise tools; MkDocs and the Material theme are pinned in a requirements file under `docs/`; the docs tasks run MkDocs through `uvx` with that file. No `pyproject.toml` at the root.
- Tasks: `docs-serve`, `docs-build` (strict), `docs-gen`, `docs-gen-check`, `docs-check`. Only `docs-gen-check` joins `mise run ci`.
- `ci.yml` gains one ubuntu job, `docs`, running `docs-build` and `docs-check` after `test`. A new `pages.yml` workflow builds and deploys on pushes to `main`, using the official Pages actions pinned to commit SHAs with version comments, with the minimal `pages: write` and `id-token: write` permissions. Enabling Pages with the "GitHub Actions" source in the repository settings is a one-time human step.
- Dependabot keeps covering the new action pins.

### Repository changes

- README shrinks to: one-paragraph description, install, one `convert` and one `filter` example, link to the site, licence and credit. The Documentation, Library quick-start and Developing sections move to the site or to contributor docs.
- MIGRATION.md and CONFORMANCE.md stay at the root, are not linked from the site, and are deleted when the last C feature is ported; a bead records that trigger. CLAUDE.md and AGENTS.md keep pointing at them until then.
- GLOSSARY.md gains Criterion and Output option (done in the grilling session). The site's reader glossary is a separate, shorter page written for users; GLOSSARY.md stays the developer vocabulary.

## Testing Decisions

A good test here exercises the published artefact or the real binary, not the generator's internals. Three checks, all at the outermost seam, and no new Go unit tests:

- **Generated reference drift** (seam: the built CLI). `docs-gen-check` rebuilds the reference with the tagged binary and diffs against the committed pages. This is the same seam the golden conformance tests use (driving the cobra root with arguments), one step further out.
- **Tutorial output** (seam: the built CLI plus the sample file). `docs-check` runs each tutorial command exactly as the reader would and compares stdout byte-for-byte with the shown block. Prior art: `scripts/difftest.sh` and the golden harness, which already hold emitted output to byte parity.
- **Site integrity** (seam: the MkDocs build). `mkdocs build --strict` fails on broken internal links, pages missing from the nav, and unknown references.

Guides are hand-verified at writing time and not automated; a follow-up issue can extend `docs-check` to them if drift appears.

## Out of Scope

- Versioned documentation (mike) and a custom domain.
- Documenting unported pgn-extract features, duplicate detection, or engine analysis beyond the "not yet" line.
- Teaching chess notation or the PGN standard.
- Publishing ADRs, design specs, MIGRATION.md or CONFORMANCE.md on the site.
- Any change to CLI behaviour. Only `Long`, `Example` and flag help *text* may change, and only in ways that do not alter parsing.
- Automated verification of guide output.
- A contributor section on the site.

## Further Notes

- Order of work: tooling and deploy first (nothing else is visible until the site builds), then the generated reference, then the tutorial with its check, then the reference grammar pages, guides, explanation pages, lineage and credits, library and glossary pages, and finally the README shrink.
- Enabling GitHub Pages is the only step an agent cannot do; the epic notes it.
- The vendored Fischer file carries the C test corpus's licence; the credits page names its origin.
