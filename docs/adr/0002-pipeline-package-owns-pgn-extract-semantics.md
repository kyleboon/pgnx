# 0002: A pipeline package owns pgn-extract's game semantics; the parser preserves the source

Status: accepted (2026-11-02)

pgn-extract corrects a game inside its parser (`grammar.c`: Result and SetUp
fix-ups, FEN consistency) and judges it in `deal_with_game`. pgnx's `convert`
and `filter` each hand-copied that sequence and had already drifted from each
other and from the oracle. We decided to split the two concerns: the `pgn`
package keeps **source fidelity** (it never rewrites a game; its only skips
are structural and sit behind `ParserOptions`), and a seventh library package,
`pipeline`, owns pgn-extract's semantics in `deal_with_game` order:
normalisation, replay, disposition (rejected / broken / ok), ECO
classification, criteria. It is built once per run and called once per game
(`pipeline.New(Config).Process(g) Outcome`); every command, and the planned
`analyze`, is a caller. `Process` touches only its game and read-only
configuration, so it is safe to call concurrently; everything order-sensitive
(tag-registry observation, selection, routing, writing) stays on the caller's
side.

## Considered options

- **Normalise in the parser, as the C tool does.** Rejected: it breaks source
  fidelity, and FEN validation needs `chess`, which `pgn` must not import.
- **Normalise in `replay`.** Rejected: replay is chess, not PGN semantics.
- **A cmd-internal pipeline.** Rejected: library users and `analyze` would
  hand-copy the loop again, which is the drift this decision removes.

## Consequences

- `output` trusts the tags it is given; its Result normalisation is deleted.
- `filter.Evaluate` no longer normalises Result or checks FEN consistency.
  The invalid-Result-becomes-"*" rule stays at its `apply.c` point inside the
  criteria stage, after tag matching, so `--tag` criteria see what the oracle
  sees.
- The Chess960 `Variant` tag belongs to normalisation once verified against
  the oracle.
- A tags-only game (tags, no moves, no result) is skipped by the parser
  behind `ParserOptions.SkipTagsOnly`, which the CLI sets, so counts and
  numbering match the oracle while library callers keep the record.
