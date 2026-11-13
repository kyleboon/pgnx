# 0005: The chess package represents the board as bitboards behind an unchanged API

Status: proposed (2026-11-13, pgnx-perf.9); accepted when the first code
commit lands

pgn-extract keeps its board as a two-dimensional array and finds attacks
and candidate moves by walking it (map.c `king_is_in_check`,
`find_knight_moves` and friends). pgnx's `chess` package was ported in
that shape, an `[8][8]Piece` mailbox probed square by square, and after
the `pgnx-perf` work up to pgnx-perf.4 it matches the C tool's wall time
on the ADR 0003 slice (1.01x). What the profile still shows is the
mailbox itself: `IsAttacked` is 17% of sampled user CPU, 84% of its calls
fall through all 18 knight, king and pawn probes into the eight ray
walks, and every probe pays a `Square.Valid()` bounds test on a
`{File, Rank int}` pair.

We decided to move the board's representation to bitboards, one 64-bit
mask per colour and piece type, with slider attacks by magic bitboards,
rather than keep narrowing the probes on the mailbox (pin rays, a
move-aware check test, slider-presence masks). On bitboards an attack
query is a handful of table lookups and the SAN-directed candidate
generation of pgnx-perf.1 is a mask intersection, so every remaining
check and legality cost disappears at once instead of one special case
at a time; the mailbox alternatives each bought a slice of the same cost
and each added a rule that the full scan had made unnecessary.

The representation is private. `Square`, `Piece`, `Move`, `Color`,
`PieceType`, `CastlingRights`, the Board's exported fields and every
exported method keep their definitions, and `Square` converts to a
0..63 index at the method boundary. The chess package is the one piece
of pgnx that is independently usable and nothing outside it should
notice the change; the multiply-add per call is the price of that, and
`Move` can shrink later if a profile ever points at it.

## Considered options

- **Narrow the probes on the mailbox.** Pin-ray legality for non-king
  moves, a check test that reuses the move just made, slider masks to
  skip ray walks. Each is bounded and oracle-safe, and together they
  would have carried the parity target. Rejected because they leave
  the probe itself expensive and add logic that bitboards make
  unnecessary.
- **A padded mailbox** (10x12 or 0x88) with precomputed deltas, which
  removes the bounds test without changing the algorithms. Rejected as
  a half step: the ray walks remain.
- **Change `Square` to an index type** with `File()` and `Rank()`
  methods. Faster at the boundary but a breaking change in every
  package and every test. Rejected in favour of converting at the
  boundary.
- **Slider attacks by classical ray lookups** (ray mask and occupancy,
  first blocker by bit scan) instead of magic bitboards: no large
  tables, a few times slower than magic. Rejected because the parity
  margin is the point of the rewrite; the magic constants are a
  published set and the tables are filled at package init. If a cold
  `pgnx --help` shows the fill above a few milliseconds, the tables
  fill lazily instead.

## Consequences

- Legality stays copy-and-probe, the C tool's `exclude_checks`
  structure: copy the board, apply the candidate, test the king. A
  pin-and-checker generator that emits only legal moves is a later
  step, taken only if the profile still shows legality probes as a top
  share.
- `LegalMoves` order is unspecified except that a pawn's promotions to
  one square are adjacent. Nothing that reaches the output depends on
  order (an ambiguous SAN is an error, not a first match); the
  evaluation output's promotion dedup counts distinct from/to pairs,
  and move-generation tests compare sets.
- The mailbox engine is the oracle for the rewrite. It stays under a
  test-only path until the bitboard engine lands, and a differential
  test replays every position of the replay bench corpus and the
  golden corpus with both, comparing legal-move sets, `IsAttacked` on
  every square for both colours, `InCheck`, `FEN`, `PlacementKey` and
  `WeakHash`. Perft gains Chess960 positions with known counts, which
  the suite had none of.
- ADR 0003's baseline table gains a row when the rewrite lands.
