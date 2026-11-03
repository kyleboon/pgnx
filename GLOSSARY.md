# pgnx

A Go port of pgn-extract: a library and CLI that parses, replays, selects, classifies and re-emits chess games in PGN. The C tool is the behavioural oracle; this glossary names the concepts pgnx uses to talk about one game's journey through a run.

## Language

### A game's journey

**Parsed game**:
A game exactly as it was written in the input: tags in source order, SAN as spelled, comments, NAGs and variations preserved. Nothing has been corrected or judged yet.
_Avoid_: raw game, input game

**Item**:
One step of a run's game iterator: a parsed game, numbered across all inputs, with the tokens the parser skipped from it; or the failure of one source. The iterator reads nothing beyond the item it is yielding.
_Avoid_: entry, event, record (that is what the report does with a verdict)

**Source fidelity**:
The parser's guarantee that a parsed game reflects the text it came from. Rewriting a game is never the parser's job.

**Annotation**:
One token written after a move: a NAG, a comment or a variation. A move keeps its annotations in the order they were written, and a comment after a variation's closing paren is an annotation of the move before the variation, following it. Comments pgnx generates for a move (a match mark) are inserted after its leading comments, before its first NAG or variation, where pgn-extract puts them.
_Avoid_: suffix comment, NAG comment (both are positions in the one list, not kinds)

**Normalisation**:
The corrections pgn-extract applies to a game before judging it: short or missing Result tags filled in, a FEN game given its SetUp tag, inconsistent SetUp/FEN tags grounds for rejection. Runs once per game, before replay, for every command.
_Avoid_: fix-up, consistency check, sanitising

**Replay**:
Playing a parsed game's moves out on a board to resolve each SAN move against the legal moves and record the positions reached.
_Avoid_: validation (that is the `validate` command's name, not the act)

**Pipeline**:
The fixed sequence a game goes through between parsing and writing: normalisation, replay, broken-ness, ECO classification, criteria, transforms. The same sequence for every command; commands differ only in what they do with the outcome.
_Avoid_: per-game loop, callback, deal_with_game

**Criterion**:
One condition a game must satisfy to be selected: a tag comparison, a length bound, an end state, a material balance, a position reached or a move sequence played. A run's criteria together decide whether a game is matched.
_Avoid_: filter (that is the command), rule, predicate, query

**Transform**:
A rewrite of a game on its way out, after it has been judged: plies dropped from its start, tags computed from it (PlyCount, HashCode) or looked up for its players, a line-number comment, or the game split into its main line and one game per variation. Changes what the emitted game holds (or how many games it is emitted as), never whether it is selected. Options that only change how a game is rendered (ply limit, FEN comments, no tags) are output options, not transforms.
_Avoid_: filter (that selects), post-processing, rewrite_game

**Output option**:
A setting that changes only how an emitted game is rendered: its notation, line width, ply limit, per-move comments, or which tags and annotations are shown. Never changes what the game holds or whether it is selected.
_Avoid_: transform (that rewrites the game), format flag, display flag

**Outcome**:
What the pipeline concludes about one game: its disposition, its reason if any, whether it matched the criteria, and the game and replay to emit.
_Avoid_: result (reserved for the chess result)

**Route**:
A command's decision for one processed game that was not rejected: which destination it goes to (the output, the unmatched output, or nowhere) and whether the run stops after it. convert routes everything to the output, validate routes nowhere, filter routes by the Selector.
_Avoid_: dispatch, callback

**Verdict**:
A command's judgement on one game once it has been routed and written: the faults the report logs and counts against it, and whether the run stops after it. The outcome is the pipeline's; the verdict is the command's.
_Avoid_: result, status (that is the disposition)

### Dispositions

**Rejected game**:
A game the pipeline refuses to emit anywhere: SetUp "1" with no FEN, an unparsable FEN, a replay failure when broken games are not kept, or a result disagreement the run was told to refuse. Counts as a failure.
_Avoid_: dropped, skipped (skipped is the parser's word for tokens it discarded), invalid

**Broken game**:
A game whose moves did not fully replay but which the run was told to keep. It is emitted only in Source format and counts as a failure.
_Avoid_: invalid game, unreplayable game

**Warning**:
A fault that is logged and makes the game count as a failure without changing whether or how it is emitted: moves dropped for a missing result token, tokens the parser discarded.
_Avoid_: problem, soft error

**Matched / unmatched game**:
Whether a game satisfied the run's criteria. A run with no criteria matches every game it does not reject.

### Positions and counting

**Ply number**:
The absolute, one-based number of the move about to be played from a position, as pgn-extract's `play_moves` counts: 1 at the standard starting position, and for a FEN start derived from the FEN's move number and side to move.
_Avoid_: half-move index, depth

**Null move**:
A ply on which the side to move passes, written `--` (pgn-extract also reads `Z0`). Replay applies it as the C tool does: the turn changes and nothing else on the board does, en passant square and clocks included. Always allowed inside a variation; allowed on the main line only by the `--allow-null-moves` option, otherwise the game is broken.
_Avoid_: pass move, skip, 0000 (the UCI protocol's spelling, which no output format uses)

**Source format**:
Output that reproduces the game's moves as written, needing no replay. The only format a broken game can be emitted in.

**Destination**:
Where a command sends emitted games: one output, the unmatched output, or the per-ECO-prefix files of a split run.
_Avoid_: writer (the thing that renders), sink

**Tag roster**:
The seven standard tags every emitted game carries in fixed order, with defaults supplied when the game lacks them.

**Argument file**:
A file of command-line arguments, one per line, that `@FILE` on the command line stands for. Expanded before the command line is parsed, so it may hold the subcommand, flags, input files or further argument files; it is pgnx's form of pgn-extract's `-A` argsfile and does not share its grammar.
_Avoid_: argsfile (the C file format), config file, options file
