package filter

import (
	"strings"
	"testing"
)

func TestValidateCheckmateExclusiveWithOtherEndStates(t *testing.T) {
	if err := (&Options{Checkmate: true}).Validate(); err != nil {
		t.Fatalf("checkmate alone: %v", err)
	}
	if err := (&Options{Stalemate: true, Insufficient: true}).Validate(); err != nil {
		t.Fatalf("stalemate with insufficient: %v", err)
	}
	for _, o := range []*Options{
		{Checkmate: true, Stalemate: true},
		{Checkmate: true, Insufficient: true},
	} {
		if err := o.Validate(); err == nil {
			t.Errorf("%+v: want error", o)
		}
	}
}

func TestValidateRatedWinnerExclusive(t *testing.T) {
	if err := (&Options{HigherRatedWinner: true}).Validate(); err != nil {
		t.Fatalf("higher alone: %v", err)
	}
	if err := (&Options{HigherRatedWinner: true, LowerRatedWinner: true}).Validate(); err == nil {
		t.Error("both: want error")
	}
}

func TestValidateBounds(t *testing.T) {
	var ok Bounds
	ok.SetMinMoves(10)
	ok.SetMaxMoves(10)
	if err := (&Options{Bounds: ok}).Validate(); err != nil {
		t.Fatalf("exactly 10 moves: %v", err)
	}
	if err := (&Options{Bounds: Bounds{MinPly: 40}}).Validate(); err != nil {
		t.Fatalf("lower bound only: %v", err)
	}
	var inverted Bounds
	inverted.SetMinMoves(10)
	inverted.SetMaxMoves(5)
	for _, b := range []Bounds{inverted, {MinPly: -1}, {MaxPly: -3}} {
		if err := (&Options{Bounds: b}).Validate(); err == nil {
			t.Errorf("%+v: want error", b)
		}
	}
}

func TestSetNMoveRuleConflict(t *testing.T) {
	// argsfile.c: --fifty then --seventyfive "conflicts with a previous
	// setting"; repeating the same rule is fine.
	o := &Options{}
	if err := o.SetNMoveRule(50); err != nil || o.NMoveRule != 50 {
		t.Fatalf("first: %v, NMoveRule %d", err, o.NMoveRule)
	}
	if err := o.SetNMoveRule(50); err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if err := o.SetNMoveRule(75); err == nil || o.NMoveRule != 50 {
		t.Errorf("change: err %v, NMoveRule %d", err, o.NMoveRule)
	}
	if err := (&Options{}).SetNMoveRule(40); err == nil {
		t.Error("40 is not a rule: want error")
	}
}

func TestValidateNMoveRule(t *testing.T) {
	for _, n := range []int{0, 50, 75} {
		if err := (&Options{NMoveRule: n}).Validate(); err != nil {
			t.Errorf("%d: %v", n, err)
		}
	}
	for _, n := range []int{-1, 40, 125} {
		if err := (&Options{NMoveRule: n}).Validate(); err == nil {
			t.Errorf("%d: want error", n)
		}
	}
}

func TestValidateCountsAndSequences(t *testing.T) {
	seq, err := ParseSequence("e4 e5")
	if err != nil {
		t.Fatal(err)
	}
	if err := (&Options{Repetition: 3, PieceCount: 7, StartPly: 5, Sequences: []*Sequence{seq}}).Validate(); err != nil {
		t.Fatalf("valid counts: %v", err)
	}
	for _, o := range []*Options{
		{Repetition: -1},
		{PieceCount: -1},
		{StartPly: -1},
		{Sequences: []*Sequence{seq, nil}},
	} {
		if err := o.Validate(); err == nil {
			t.Errorf("%+v: want error", o)
		}
	}
}

func TestPositionCriteriaValidateMatchPlyLimit(t *testing.T) {
	// argsfile.c: a --matchplylimit below the depth the -x lines imply
	// "conflicts with existing higher limit", whichever came first.
	pc := lineCriteria(t, "e4 c5 Nf3 d6 d4 cxd4") // depth 6+8 = 14
	pc.MatchPlyLimit = 14
	if err := pc.Validate(); err != nil {
		t.Fatalf("limit at depth: %v", err)
	}
	pc.MatchPlyLimit = 5
	err := pc.Validate()
	if err == nil || !strings.Contains(err.Error(), "5") || !strings.Contains(err.Error(), "14") {
		t.Errorf("limit below depth must quote both numbers: %v", err)
	}
	if err := (&PositionCriteria{MatchPlyLimit: -1}).Validate(); err == nil {
		t.Error("negative limit: want error")
	}
	if err := (&PositionCriteria{MatchPlyLimit: 5}).Validate(); err != nil {
		t.Errorf("limit without lines: %v", err)
	}
	var nilPC *PositionCriteria
	if err := nilPC.Validate(); err != nil {
		t.Errorf("nil: %v", err)
	}
}

func TestValidateDelegatesToPositions(t *testing.T) {
	pc := lineCriteria(t, "e4 c5 Nf3 d6 d4 cxd4")
	pc.MatchPlyLimit = 5
	if err := (&Options{Positions: pc}).Validate(); err == nil {
		t.Error("want the positional error")
	}
}

func TestSelectorOptionsValidate(t *testing.T) {
	if err := (SelectorOptions{StopAfter: 2, FirstGame: 3, GameLimit: 9}).Validate(); err != nil {
		t.Fatalf("valid: %v", err)
	}
	if err := (SelectorOptions{FirstGame: 3}).Validate(); err != nil {
		t.Fatalf("first game without limit: %v", err)
	}
	for _, o := range []SelectorOptions{
		{StopAfter: -1},
		{FirstGame: -1},
		{GameLimit: -1},
		{FirstGame: 5, GameLimit: 2},
	} {
		if err := o.Validate(); err == nil {
			t.Errorf("%+v: want error", o)
		}
	}
}
