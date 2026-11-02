package filter

import "testing"

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
