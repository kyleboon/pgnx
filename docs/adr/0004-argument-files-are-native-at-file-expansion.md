# 0004: Argument files are a pgnx-native `@FILE` expansion, not a reader of C argsfiles

Status: accepted (2026-11-03; beads `pgnx-argsfile`)

pgn-extract's `-A FILE` reads further arguments from a file in a grammar of
its own (`argsfile.c`): `:-X` lines carry a C flag and its value, `:-t`,
`:-v`, `:-x`, `:-z`, `:-y` and `:-R` lines switch the following bare lines
into tag criteria, move sequences, positions, material patterns or a roster,
and `:file.pgn` names an input. pgnx's CLI is redesigned around subcommands
and long flags, so none of those flags exist here as written. We decided
that pgnx has its own argument files: any argument `@FILE`, on the command
line or inside another argument file, is replaced before cobra parses the
line by the arguments read from FILE, one per line, a flag sharing its line
with its value. The grammar is documented in MIGRATION.md ("Argument
files") and the root command's help; `cmd/pgnx/argfile.go` implements it
and `runRoot` is the single entry point that applies it.

## Considered options

- **Read C argsfiles verbatim, translating each C flag through the
  MIGRATION table.** Rejected: it would make pgnx carry a second, C-shaped
  command-line parser (short-letter flags with glued values, the
  mode-switching sections, `-s`, `-d`) that must track two CLIs at once and
  would still break on every flag pgnx renders differently or has not
  ported. MIGRATION.md is the translation, and it is a one-time act per
  file.
- **A `--args-file FILE` flag.** Rejected: the file may hold the subcommand
  and any subcommand's flags, so expansion has to run before cobra sees the
  arguments, where recognising a flag and its value forms (`--args-file X`,
  `--args-file=X`, `-A X`, `-AX`) would mean re-parsing flags ourselves.
  `@FILE` is one token, needs no parsing, and is the convention of gcc,
  javac, 7z and friends.
- **One shell-quoted command line per file, split like a shell.** Rejected:
  tag criteria carry their own double quotes (`White "Fischer"`), which a
  shell-style splitter would eat or force the user to escape; pgn-extract's
  own files never need quoting and neither should ours.

## Consequences

- Grammar: one argument per line; a line starting with `-` splits at its
  first white space into flag and value; blank lines and comment lines
  (first non-blank character `%`) are ignored; paths are relative to the
  working directory; `--` ends expansion. Nested `@FILE` lines are
  allowed and a cycle is an error.
- Expansion is textual and runs before parsing, so an argument file cannot
  do anything the command line cannot, and parity with the C oracle is a
  property of the expanded arguments: the `test-FENPattern` golden target
  runs both ways.
- `execute` and `runRoot` take the argument list; tests that drive the CLI
  end to end go through them, since `root.Execute()` alone skips expansion.
- A C argsfile is not accepted. CONFORMANCE.md records the divergence and
  MIGRATION.md shows the translation; `test-AA` waits for duplicate
  detection, not for this.
