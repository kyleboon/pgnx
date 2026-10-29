package filter

import "testing"

func TestBoundsConversion(t *testing.T) {
	var b Bounds
	b.SetMinMoves(45)
	if b.MinPly != 89 || b.MaxPly != 0 {
		t.Errorf("-bl45: %+v", b)
	}
	b = Bounds{}
	b.SetMaxMoves(45)
	if b.MinPly != 0 || b.MaxPly != 90 {
		t.Errorf("-bu45: %+v", b)
	}
	b = Bounds{}
	b.SetMoves(30)
	if b.MinPly != 59 || b.MaxPly != 60 {
		t.Errorf("-b30: %+v", b)
	}
	b = Bounds{}
	b.SetMinPly(90)
	if b.MinPly != 90 || b.MaxPly != 0 {
		t.Errorf("-pl90: %+v", b)
	}
	b = Bounds{}
	b.SetMaxPly(90)
	if b.MinPly != 0 || b.MaxPly != 90 {
		t.Errorf("-pu90: %+v", b)
	}
	b = Bounds{}
	b.SetPly(60)
	if b.MinPly != 60 || b.MaxPly != 60 {
		t.Errorf("-p60: %+v", b)
	}
}

func TestBoundsMatch(t *testing.T) {
	b := Bounds{MinPly: 59, MaxPly: 60}
	for ply, want := range map[int]bool{58: false, 59: true, 60: true, 61: false} {
		if got := b.Match(ply); got != want {
			t.Errorf("Match(%d) = %v", ply, got)
		}
	}
	if !(Bounds{}).Match(0) || !(Bounds{}).Match(100000) {
		t.Error("zero bounds match everything")
	}
	if (Bounds{MinPly: 1}).Match(0) {
		t.Error("min bound")
	}
}
