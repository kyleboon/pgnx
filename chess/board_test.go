package chess

import "testing"

// NewBoard hands out the standard starting position copied from one
// parsed once, so each board is the caller's own: mutating one leaves
// the next untouched.
func TestNewBoardIsIndependent(t *testing.T) {
	a := NewBoard()
	a.Apply(Move{From: MustSquare("e2"), To: MustSquare("e4")})
	b := NewBoard()
	if got, want := b.FEN(), startPositionFEN; got != want {
		t.Fatalf("NewBoard after mutating an earlier one = %s, want %s", got, want)
	}
	if n := testing.AllocsPerRun(100, func() { NewBoard() }); n > 1 {
		t.Errorf("NewBoard allocated %v objects, want 1 (a copy of the parsed start board)", n)
	}
}
