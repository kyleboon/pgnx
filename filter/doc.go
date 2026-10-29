// Package filter implements game selection for pgnx: tag criteria
// (pgn-extract's -t/-T family), length bounds (-b/-p), end-state and
// game-property predicates (--checkmate, --fifty, --repetition, ...),
// material-balance matching (-z/-y), result-tag consistency
// (--nobadresults/--fixresulttags), and the matched-game bookkeeping
// behind --selectonly, --skipmatching, --stopafter, --firstgame and
// --gamelimit.
//
// The predicates are pure functions over a parsed *pgn.Game and its
// *replay.Result. Evaluate runs them in the same order as the C tool's
// deal_with_game chain (grammar.c) so that the observable side effects
// (which games reach which output, where match-marking comments land,
// what the Result tag looks like when --result criteria are tested) are
// identical. Evaluate's documented mutations of the game are the only
// ones this package performs.
//
// filter imports chess, pgn and replay and never writes output; the CLI
// composes Evaluate and Selector with output.Writer.
package filter
