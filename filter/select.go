package filter

import (
	"fmt"
	"strconv"
	"strings"
)

// Range is an inclusive range of matched-game ordinals.
type Range struct {
	First, Last int
}

// ParseRanges parses a --selectonly/--skipmatching list (argsfile.c
// extract_game_number_list): comma-separated N or N1:N2 entries, all
// positive, strictly ascending and non-overlapping.
func ParseRanges(s string) ([]Range, error) {
	var out []Range
	last := 0
	for _, tok := range strings.Split(s, ",") {
		var r Range
		var err error
		if i := strings.IndexByte(tok, ':'); i >= 0 {
			r.First, err = strconv.Atoi(strings.TrimSpace(tok[:i]))
			if err == nil {
				r.Last, err = strconv.Atoi(strings.TrimSpace(tok[i+1:]))
			}
		} else {
			r.First, err = strconv.Atoi(strings.TrimSpace(tok))
			r.Last = r.First
		}
		if err != nil || r.First <= last || r.First > r.Last {
			return nil, fmt.Errorf("game numbers in %q must be N or N:N, positive and in ascending order", s)
		}
		last = r.Last
		out = append(out, r)
	}
	return out, nil
}

// SelectorOptions configures which matched games are emitted and when
// processing stops.
type SelectorOptions struct {
	// SelectOnly emits only the matched games whose match ordinal falls in
	// these ranges (--selectonly); SkipMatching suppresses those that do
	// (--skipmatching). Suppressed games go to neither output.
	SelectOnly, SkipMatching []Range
	// StopAfter stops once this many games have matched (--stopafter);
	// 0 means never.
	StopAfter int
	// FirstGame is the 1-based index of the first game that may match
	// (--firstgame); earlier games count as unmatched. 0 or 1 means all.
	FirstGame int
	// GameLimit stops after this many games have been processed
	// (--gamelimit); 0 means never.
	GameLimit int
}

// Decision is Selector.Decide's verdict for one game.
type Decision struct {
	Emit        bool // write to the matched output
	ToUnmatched bool // write to the unmatched output, if any
	Stop        bool // do not read any further games
}

// Selector is the per-run bookkeeping behind SelectorOptions. It is the
// only cross-game state in the package.
type Selector struct {
	opts      SelectorOptions
	matched   int
	nextOut   int // index into opts.SelectOnly
	nextSkip  int // index into opts.SkipMatching
	processed int
}

// NewSelector returns a Selector for o.
func NewSelector(o SelectorOptions) *Selector {
	return &Selector{opts: o}
}

// Matched returns how many games have matched so far (counting those
// held back by SelectOnly/SkipMatching, but not those before FirstGame).
func (s *Selector) Matched() int { return s.matched }

// Decide records game number gameIndex (1-based, across all inputs) with
// its Evaluate result and says what to do with it (grammar.c
// deal_with_game and finished_processing). Stop is reported with the
// game that triggers it; the C tool instead parses one more game and
// discards it, which is invisible in the output.
func (s *Selector) Decide(gameIndex int, matched bool) Decision {
	s.processed = gameIndex
	var d Decision
	if matched && gameIndex >= s.opts.FirstGame {
		s.matched++
		switch {
		case s.opts.SelectOnly != nil && !s.inRange(s.opts.SelectOnly, s.nextOut):
			// Not a selected match: emitted nowhere.
		case s.opts.SkipMatching != nil && s.inRange(s.opts.SkipMatching, s.nextSkip):
			if s.matched == s.opts.SkipMatching[s.nextSkip].Last {
				s.nextSkip++
			}
		default:
			d.Emit = true
		}
		if s.opts.SelectOnly != nil && s.inRange(s.opts.SelectOnly, s.nextOut) &&
			s.matched == s.opts.SelectOnly[s.nextOut].Last {
			s.nextOut++
		}
	} else {
		d.ToUnmatched = true
	}
	d.Stop = s.finished()
	return d
}

func (s *Selector) inRange(ranges []Range, i int) bool {
	return i < len(ranges) && ranges[i].First <= s.matched && s.matched <= ranges[i].Last
}

func (s *Selector) finished() bool {
	return (s.opts.SelectOnly != nil && s.nextOut >= len(s.opts.SelectOnly)) ||
		(s.opts.StopAfter > 0 && s.matched == s.opts.StopAfter) ||
		(s.opts.GameLimit > 0 && s.processed >= s.opts.GameLimit)
}
