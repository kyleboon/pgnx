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
