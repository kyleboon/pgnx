package chess

import "testing"

func TestSquareRoundTrip(t *testing.T) {
	for _, name := range []string{"a1", "e4", "h8"} {
		sq, err := ParseSquare(name)
		if err != nil {
			t.Fatalf("ParseSquare(%q): %v", name, err)
		}
		if got := sq.String(); got != name {
			t.Errorf("round trip %q = %q", name, got)
		}
	}
}

func TestParseSquareRejectsBad(t *testing.T) {
	for _, name := range []string{"", "i1", "a9", "e", "e44"} {
		if _, err := ParseSquare(name); err == nil {
			t.Errorf("ParseSquare(%q): want error", name)
		}
	}
}

func TestOpponent(t *testing.T) {
	if White.Opponent() != Black || Black.Opponent() != White {
		t.Fatal("Opponent broken")
	}
}

func TestZeroPieceIsEmpty(t *testing.T) {
	var p Piece
	if p.Type != NoPiece || p.Color != NoColor {
		t.Fatal("zero Piece must be empty")
	}
}
