package filter

import (
	"errors"
	"fmt"

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

// Validate reports the first way in which o is not a usable set of
// criteria (the rules pgn-extract's argument handling enforces before
// any game is read). The command layer maps a non-nil error to a usage
// error; a zero Options is valid.
func (o *Options) Validate() error {
	if o.Checkmate && (o.Stalemate || o.Insufficient) {
		return errors.New("checkmate cannot be combined with stalemate or insufficient material")
	}
	if o.HigherRatedWinner && o.LowerRatedWinner {
		return errors.New("higher-rated winner and lower-rated winner are mutually exclusive")
	}
	if o.Bounds.MinPly < 0 || o.Bounds.MaxPly < 0 {
		return errors.New("length bounds cannot be negative")
	}
	if o.Bounds.MaxPly != 0 && o.Bounds.MinPly > o.Bounds.MaxPly {
		return fmt.Errorf("lower length bound of %d plies exceeds the upper bound of %d", o.Bounds.MinPly, o.Bounds.MaxPly)
	}
	return nil
}

// Evaluate reports whether g (with its replay r, which may be partial
// for a broken game kept with --keep-broken) satisfies o, running the
// checks in the same order as pgn-extract's deal_with_game chain
// (grammar.c:1214, apply.c:955-1330) and stopping at the first failure.
// Evaluate trusts the tags it is given: the C parser's corrections
// (Result "1/2" → "1/2-1/2", absent/empty/"?" Result filled from the
// terminating token, SetUp "1" added to a FEN game, inconsistent
// SetUp/FEN tags rejected) are pipeline.Normalise's job, run once per
// game before replay, so tag criteria see what the oracle's do.
//
//  1. Tag criteria other than ECO, then higher/lower-rated-winner.
//  2. SetUp-tag filter.
//  3. ECO classification is not Evaluate's job: the CLI classifies a
//     replayed game (replacing its ECO/Opening/Variation/SubVariation
//     tags) before calling Evaluate, which is observably the same as the
//     C tool classifying inside apply_move_list.
//  4. An invalid Result tag becomes "*" (the first line of apply.c
//     play_moves and check_move_validity), after the tag checks have
//     seen it as written but even when they failed, since the C tool's
//     non-matching branch replays such a game before writing it to -n.
//     Then the replay-time walk (play_moves) over the main line and,
//     unless NoVariations, its variations, each main-line ply having the
//     absolute ply number p (1 for a standard start; from a FEN,
//     2*(Fullmove-1)+(Black?1:0)+1 for the first move). When positions
//     are sought: the start position is
//     tested before any move when p >= StartPly, a match going to the
//     front of the prefix comments; variations on a move are walked
//     first, each with a fresh matched state, a match inside marking the
//     variation move and matching the game without contributing a label;
//     the position after a move is tested when p >= StartPly and nothing
//     has matched yet, a match being marked on that move before any
//     repetition or N-move mark at the same ply and its label recorded
//     (main line only); the walk stops once p exceeds the search depth
//     (MatchPlyLimit, or the longest -x line) without a match, leaving
//     the game unmatched with no later marks.
//  5. Repetition and N-move-rule marks, and the underpromotion check,
//     only from StartPly and only as far as the walk went.
//  6. A MatchLabel tag when AddLabelTag and the label is non-empty.
//  7. The end-of-game result check, if the walk reached the last move
//     (FixResultTags may rewrite the Result tag and g.Result; NoBadResults
//     may return ErrBadResult).
//  8. Positions sought but unmatched, the N-move rule, underpromotion,
//     stalemate and insufficient material reject the game.
//  9. Ply-count bounds.
//  10. Move sequences (check_textual_variations): any one of Sequences,
//     straight or permuted, from the first move or, with
//     SequenceAnywhere, from any move.
//  11. Material patterns, then piece count (each may insert a mark).
//  12. Checkmate, then repetition.
//  13. ECO tag criteria (last, after classification has had its say).
//  14. Commented.
//
// Evaluate mutates g only in the ways listed: the Result tag/token,
// mark comments, and the MaterialMatch and MatchLabel tags. Because
// marks and those tags are added before later criteria are tested, a
// game that is marked and then rejected (by bounds, say) keeps its marks
// and is seen that way by an unmatched-games output, exactly as in C;
// conversely a game rejected by an earlier step (a failed sequence, say)
// never receives a mark from a later one.
func Evaluate(g *pgn.Game, r *replay.Result, o *Options) (bool, error) {
	if o == nil {
		o = &Options{}
	}
	if r == nil {
		r = &replay.Result{}
	}

	tagsMatch := o.Tags == nil || o.Tags.Match(g)
	if tagsMatch && (o.HigherRatedWinner || o.LowerRatedWinner) && !RatedWinner(g, o.HigherRatedWinner, o.LowerRatedWinner) {
		tagsMatch = false
	}
	if _, hasSetup := g.Tag("SetUp"); tagsMatch && ((o.Setup == OnlySetup && !hasSetup) || (o.Setup == NoSetup && hasSetup)) {
		tagsMatch = false
	}

	// apply_move_list / play_moves. The Result rewrite happens whether
	// or not the tag checks passed: C's non-matching branch forces
	// apply_move_list (check_move_validity, which repeats the rule) on a
	// game that failed them before writing it to the -n file, so every
	// game that reaches an output has a valid Result tag.
	ensureValidResultTag(g)
	if !tagsMatch {
		return false, nil
	}
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
		g.SetTag("MatchLabel", label)
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
