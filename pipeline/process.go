package pipeline

import (
	"errors"
	"fmt"

	"github.com/kyleboon/pgnx/eco"
	"github.com/kyleboon/pgnx/filter"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
	"github.com/kyleboon/pgnx/transform"
)

// Config is a run's per-game configuration, fixed before the first game
// and read-only afterwards.
type Config struct {
	// ECO, when non-nil, classifies every replayed game, replacing its
	// ECO, Opening, Variation and SubVariation tags (-e). A broken game
	// kept with KeepBroken is classified from the plies that replayed,
	// as the C tool classifies inside apply_move_list.
	ECO *eco.Table

	// KeepBroken keeps a game whose moves do not fully replay
	// (--keep-broken): its Status is Broken rather than Rejected and it
	// is still classified and evaluated, against the partial replay.
	KeepBroken bool

	// Criteria is the run's selection criteria (filter.Options). nil is
	// the empty criteria, which every game that is not Rejected matches:
	// convert is filter with no criteria by construction. The criteria
	// stage always runs, since the replay-time tag rewrites (apply.c
	// play_moves: an invalid Result tag becomes "*") happen there in the
	// C tool.
	Criteria *filter.Options

	// Transform is the run's output-side rewrites (transform.Options):
	// ply dropping and the added tags and comments. nil runs only the
	// rewrite the C tool always does (a HashCode tag present on the
	// game is recomputed). The transforms run last, on the game as it
	// will be emitted, so the criteria see the game as parsed.
	Transform *transform.Options
}

// Pipeline runs pgn-extract's deal_with_game sequence over one game at a
// time. Build one per run with New and call Process once per game.
type Pipeline struct {
	cfg Config
}

// New returns a Pipeline for cfg. cfg must not be modified afterwards.
func New(cfg Config) *Pipeline {
	return &Pipeline{cfg: cfg}
}

// Status is a processed game's disposition (GLOSSARY.md "Dispositions").
type Status int

const (
	// OK: the game normalised and replayed in full. It is emitted in the
	// run's output format when Matched.
	OK Status = iota
	// Broken: the moves did not fully replay but KeepBroken kept the
	// game. It is emitted in Source format only, and counts as a failure.
	Broken
	// Rejected: the game is emitted nowhere, whatever the run was told:
	// SetUp "1" without a FEN tag, an unparsable FEN, a replay failure
	// without KeepBroken, or a Result disagreement the criteria refuse
	// (filter.ErrBadResult). Counts as a failure.
	Rejected
)

// String returns the status in lower case, for diagnostics.
func (s Status) String() string {
	switch s {
	case OK:
		return "ok"
	case Broken:
		return "broken"
	case Rejected:
		return "rejected"
	}
	return fmt.Sprintf("Status(%d)", int(s))
}

// Outcome is what the pipeline concluded about one game.
type Outcome struct {
	// Game is the processed game: the one passed to Process, normalised
	// and with whatever tags and marks classification and the criteria
	// added.
	Game *pgn.Game

	// Replay is the game's replay: complete when Status is OK, partial
	// (the plies before the failure) when Broken, nil when Rejected.
	Replay *replay.Result

	// Status is the disposition.
	Status Status

	// Reason says why the game is Broken or Rejected; nil iff Status is
	// OK. It is the replay failure for a Broken game.
	Reason error

	// Warnings are faults that are reported but change neither whether
	// nor how the game is emitted; the game still counts as a failure.
	// Today: the moves dropped from a game with no result token that did
	// not replay. Tokens the parser discarded are a parse fact on the
	// game (pgn.Game.SkippedTokens), not a warning here.
	Warnings []error

	// Matched reports whether the game satisfied the criteria. Always
	// false when Rejected; a Broken game is matched against its partial
	// replay.
	Matched bool
}

// ReplayToEmit is the replay the game is rendered with: the complete
// replay of an OK game; nil for a Broken game, which is emitted in
// Source format only (the one format that needs no replay), and for a
// Rejected game, which is emitted nowhere.
func (o Outcome) ReplayToEmit() *replay.Result {
	if o.Status != OK {
		return nil
	}
	return o.Replay
}

// Faults are what the game is reported for, in reporting order: its
// Reason, if any, then its Warnings. Empty iff the game succeeded.
func (o Outcome) Faults() []error {
	if o.Reason == nil {
		return o.Warnings
	}
	return append([]error{o.Reason}, o.Warnings...)
}

// Process runs the pipeline over g: Normalise; replay; decide broken-ness
// against KeepBroken; ECO classification; the criteria; the output
// transforms. It takes
// ownership of g and mutates it in place (tags, marks, and the dropped
// moves described under Outcome.Warnings); the caller reads the game
// back from the Outcome.
//
// Process touches only g and the Pipeline's read-only configuration, so
// one Pipeline may process different games from several goroutines at
// once. Everything order-sensitive (tag-registry observation, selection,
// routing, writing) is the caller's.
func (p *Pipeline) Process(g *pgn.Game) Outcome {
	o := Outcome{Game: g}
	if err := Normalise(g); err != nil {
		o.Status, o.Reason = Rejected, err
		return o
	}

	r, replayErr := replay.Game(g, replay.Options{})
	if replayErr != nil && !p.cfg.KeepBroken && g.Result == "" && len(g.Moves) > 0 {
		// pgn-extract never replays a game whose movetext has no result
		// token (grammar.c parse_game reports "Missing result" and
		// discards the moves), so it never notices they are illegal and
		// emits the tag section with the Result tag as the movetext. Do
		// the same for that case only; KeepBroken wants the moves back.
		// See CONFORMANCE.md.
		g.Moves = nil
		r, _ = replay.Game(g, replay.Options{})
		o.Warnings = append(o.Warnings, fmt.Errorf("no result token and the moves do not replay (%v); moves dropped", replayErr))
		replayErr = nil
	}
	if replayErr != nil {
		if !p.cfg.KeepBroken {
			o.Status, o.Reason = Rejected, replayErr
			return o
		}
		o.Status, o.Reason = Broken, replayErr
	}
	o.Replay = r

	// Classification precedes the criteria so ECO criteria see the new
	// tags (the C tool classifies inside apply_move_list, before
	// check_ECO_tag).
	if p.cfg.ECO != nil {
		if e, ok := p.cfg.ECO.Classify(r); ok {
			eco.Apply(g, e)
		}
	}

	matched, err := filter.Evaluate(g, r, p.cfg.Criteria)
	if err != nil {
		o.Status, o.Reason, o.Replay = Rejected, err, nil
		return o
	}
	// Output-side rewrites come last (output.c format_game is called on
	// the way out), after everything that judges the game; a game too
	// short for the plies asked to be dropped is emitted nowhere, as the
	// C tool emits nothing for it.
	var topts transform.Options
	if p.cfg.Transform != nil {
		topts = *p.cfg.Transform
	}
	// rewrite_game keeps the board under --keepbroken, so a game too
	// short for the drop still goes out, reported but not cut.
	topts.KeepShort = p.cfg.KeepBroken
	transformed, err := transform.Apply(g, r, topts)
	var short *transform.ShortGameError
	if errors.As(err, &short) && topts.KeepShort {
		o.Warnings = append(o.Warnings, err)
		err = nil
	}
	if err != nil {
		o.Status, o.Reason, o.Replay = Rejected, err, nil
		return o
	}
	r = transformed
	o.Replay = r
	o.Matched = matched
	return o
}
