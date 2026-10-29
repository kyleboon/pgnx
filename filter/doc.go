// Package filter implements game selection for pgnx: tag criteria
// (pgn-extract's -t/-T family), length bounds (-b/-p), end-state and
// game-property predicates (--checkmate, --fifty, --repetition, ...),
// material-balance matching (-z/-y), positional matching by move line,
// FEN, Polyglot hash and FEN pattern (-x, FEN lines, -H, -Tf/--fenpattern,
// with --startply, --matchplylimit, --wtm/--btm and --addlabeltag),
// move-sequence matching (-v, -P, --vanywhere),
// result-tag consistency (--nobadresults/--fixresulttags), and the
// matched-game bookkeeping behind --selectonly, --skipmatching,
// --stopafter, --firstgame and --gamelimit.
//
// A -t criteria file mixes the two kinds of criteria: ParseCriterion and
// ReadCriteria return every line as a Criterion, and the caller sends
// tag criteria to a TagMatcher and positional ones (FEN, FENPattern and
// FENPatternI lines, recognised by Criterion.IsPositional, the pattern
// forms carrying a Label) to a PositionCriteria. TagMatcher.Add refuses
// a positional criterion.
//
// The predicates are pure functions over a parsed *pgn.Game and its
// *replay.Result. Evaluate runs them in the same order as the C tool's
// deal_with_game chain (grammar.c, apply.c) so that the observable side
// effects (which games reach which output, where match-marking comments
// land, what the Result tag looks like when --result criteria are
// tested, which rejected games still carry marks) are identical; its doc
// comment lists the order. ECO classification is the CLI's step before
// Evaluate; ECO tag criteria are tested last so they see the classified
// tags. Evaluate's documented mutations of the game are the only ones
// this package performs.
//
// filter imports chess, pgn and replay and never writes output; the CLI
// composes Evaluate and Selector with output.Writer.
package filter
