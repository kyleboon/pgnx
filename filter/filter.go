package filter

import (
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// SetupFilter selects games by the presence of a SetUp tag.
type SetupFilter int

const (
	AnySetup  SetupFilter = iota // no restriction
	OnlySetup                    // --onlysetuptags: SetUp tag required
	NoSetup                      // --nosetuptags: SetUp tag forbidden
)

// Options are the selection criteria Evaluate applies. The zero value
// matches every game.
type Options struct {
	// Tags holds -t/-T criteria; nil or empty means no tag restriction.
	Tags *TagMatcher
	// HigherRatedWinner / LowerRatedWinner require the game to have been
	// won by the higher / lower rated player (both Elo tags required).
	HigherRatedWinner, LowerRatedWinner bool
	// Setup restricts games by SetUp tag presence.
	Setup SetupFilter

	// NoBadResults makes Evaluate return ErrBadResult for a game whose
	// Result tag disagrees with its terminating result. FixResultTags
	// instead rewrites the tag (and token) from the final position.
	NoBadResults, FixResultTags bool

	// Bounds restricts the ply count.
	Bounds Bounds

	// Material patterns (-z/-y); any one matching suffices. AddMatchTag
	// adds a MaterialMatch tag naming the colour that took the first set.
	Material    []*MaterialPattern
	AddMatchTag bool
	// PieceCount > 0 requires a position with exactly that many pieces.
	PieceCount int

	// Final-position predicates.
	Checkmate, Stalemate, Insufficient bool
	// NMoveRule is 50 (--fifty) or 75 (--seventyfive); 0 disables.
	NMoveRule int
	// Repetition is the fold count required (3 for --repetition, 5 for
	// --repetition5); 0 disables.
	Repetition int
	// Underpromotion requires a promotion to something other than a queen;
	// Commented requires at least one comment anywhere in the game.
	Underpromotion, Commented bool

	// Sequences holds -v move sequences, matched against the main-line
	// moves as written (see ParseSequence); any one matching suffices.
	// ExactOrder requires the listed move order (-P); SequenceAnywhere
	// lets a match start at any move rather than the first (--vanywhere).
	Sequences        []*Sequence
	ExactOrder       bool
	SequenceAnywhere bool

	// Positions holds the positional criteria (-x lines, FEN positions,
	// -H hashes, FEN patterns); nil or empty means none are sought.
	Positions *PositionCriteria
	// StartPly is the first ply number at which positional, repetition,
	// N-move-rule and underpromotion checks run (--startply); 0 means 1.
	StartPly int
	// NoVariations stops the positional search from entering variations
	// (the C tool's keep_variations, cleared by -V).
	NoVariations bool
	// AddLabelTag adds a MatchLabel tag holding the label of the FEN
	// pattern that matched (--addlabeltag); nothing is added for an
	// unlabelled pattern or a match by position or hash.
	AddLabelTag bool

	// MarkMatches, when non-empty, is inserted as a comment after every
	// main-line move at which an N-move-rule, repetition, material or
	// piece-count match occurs, and after the move reaching the first
	// positional match (--markmatches). "FEN" inserts the position's FEN
	// instead.
	MarkMatches string
}

// Evaluate reports whether g (with its replay r, which may be partial
// for a broken game kept with --keep-broken) satisfies o, running the
// checks in the same order as pgn-extract's deal_with_game chain
// (grammar.c) and stopping at the first failure:
//
//  1. Result-tag normalisation ("1/2" → "1/2-1/2"; absent/empty/"?"
//     filled from the terminating token), as the C parser does.
//  2. FEN/SetUp consistency (may add SetUp "1"; rejects SetUp without FEN).
//  3. Tag criteria, then higher/lower-rated-winner.
//  4. SetUp-tag filter.
//  5. The replay-time checks (apply.c play_moves): an invalid Result tag
//     becomes "*"; the positional walk runs over the main line and its
//     variations, marking the first match and stopping once the search
//     depth is exceeded without one; repetition and N-move-rule marks are
//     inserted (after a positional mark at the same ply); underpromotion
//     is noted; a MatchLabel tag is added for a labelled pattern match;
//     the end-of-game result check runs if the walk reached the last move
//     (FixResultTags may rewrite the Result tag and g.Result; NoBadResults
//     may return ErrBadResult); then the positional criteria, N-move
//     rule, underpromotion, stalemate and insufficient material are
//     enforced. Checks gated by StartPly only look at plies from there.
//  6. Ply-count bounds.
//  7. Move sequences (any one of Sequences, straight or permuted).
//  8. Material patterns, then piece count (each may insert a mark).
//  9. Checkmate, then repetition.
//  10. ECO tag criteria (last, so a future classifier can add the tag).
//  11. Commented.
//
// Evaluate mutates g only in the ways listed: the Result tag/token, a
// SetUp "1" tag, mark comments, and the MaterialMatch and MatchLabel
// tags. Because marks
// are inserted before later criteria are tested, a game that is marked
// and then rejected keeps its marks (and is seen that way by an
// unmatched-games output), exactly as in C.
func Evaluate(g *pgn.Game, r *replay.Result, o *Options) (bool, error) {
	if o == nil {
		o = &Options{}
	}
	if r == nil {
		r = &replay.Result{}
	}

	normaliseResult(g)
	if !consistentFENTags(g) {
		return false, nil
	}
	if o.Tags != nil && !o.Tags.Match(g) {
		return false, nil
	}
	if (o.HigherRatedWinner || o.LowerRatedWinner) && !RatedWinner(g, o.HigherRatedWinner, o.LowerRatedWinner) {
		return false, nil
	}
	if _, hasSetup := g.Tag("SetUp"); (o.Setup == OnlySetup && !hasSetup) || (o.Setup == NoSetup && hasSetup) {
		return false, nil
	}

	// apply_move_list / play_moves.
	ensureValidResultTag(g)
	startPly := o.StartPly
	if startPly < 1 {
		startPly = 1
	}
	positioned, label, stop := true, "", len(r.Moves)
	if !o.Positions.Empty() {
		positioned, label, stop = matchPositions(g, r, o.Positions, startPly, o.MarkMatches, o.NoVariations)
	}
	repeated := true
	if o.Repetition > 0 {
		repeated = matchRepetition(g, r, o.Repetition, o.MarkMatches, startPly, stop)
	}
	nMoveRule := true
	if o.NMoveRule > 0 {
		nMoveRule = matchNMoveRule(g, r, o.NMoveRule, o.MarkMatches, startPly, stop)
	}
	if o.AddLabelTag && label != "" {
		setTag(g, "MatchLabel", label)
	}
	if stop >= len(r.Moves) {
		// The result check lives at the last move of the walk; a walk cut
		// short by the search depth never reaches it.
		if err := checkResult(g, r, o.FixResultTags, o.NoBadResults); err != nil {
			return false, err
		}
	}
	if !positioned || !nMoveRule {
		return false, nil
	}
	if o.Underpromotion && !matchUnderpromotion(r, startPly, stop) {
		return false, nil
	}
	if o.Stalemate && !matchStalemate(r) {
		return false, nil
	}
	if o.Insufficient && !matchInsufficient(r) {
		return false, nil
	}

	if !o.Bounds.Match(r.PlyCount()) {
		return false, nil
	}
	if !matchSequences(g, o.Sequences, o.ExactOrder, o.SequenceAnywhere) {
		return false, nil
	}
	if len(o.Material) > 0 && !matchMaterial(g, r, o.Material, o.MarkMatches, o.AddMatchTag) {
		return false, nil
	}
	if o.PieceCount > 0 && !matchPieceCount(g, r, o.PieceCount, o.MarkMatches) {
		return false, nil
	}
	if o.Checkmate && !matchCheckmate(r) {
		return false, nil
	}
	if !repeated {
		return false, nil
	}
	if o.Tags != nil && !o.Tags.MatchECO(g) {
		return false, nil
	}
	if o.Commented && !matchCommented(g) {
		return false, nil
	}
	return true, nil
}
