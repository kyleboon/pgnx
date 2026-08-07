package chess

import "testing"

func attacked(t *testing.T, fen, sq string, by Color) bool {
	t.Helper()
	b, err := FromFEN(fen)
	if err != nil {
		t.Fatal(err)
	}
	return b.IsAttacked(MustSquare(sq), by)
}

func TestIsAttacked(t *testing.T) {
	cases := []struct {
		fen, sq string
		by      Color
		want    bool
	}{
		{"8/8/8/8/8/5N2/8/K6k w - - 0 1", "e5", White, true}, // knight
		{"8/8/8/8/8/5N2/8/K6k w - - 0 1", "e4", White, false},
		{"8/8/8/8/3r4/8/8/K2R3k w - - 0 1", "d7", Black, true},    // rook ray up
		{"8/8/8/3P4/3r4/8/8/K2R3k w - - 0 1", "d7", Black, false}, // blocked by pawn on d5
		{"8/8/8/8/8/8/6p1/K6k b - - 0 1", "f1", Black, true},      // black pawn attacks diagonally down
		{"8/8/8/8/8/8/6p1/K6k b - - 0 1", "g3", Black, false},     // pawns don't attack straight
		{"8/8/8/8/8/8/8/K1b4k b - - 0 1", "a3", Black, true},      // c1->a3 IS a diagonal
	}
	for _, c := range cases {
		if got := attacked(t, c.fen, c.sq, c.by); got != c.want {
			t.Errorf("%s attacked-by-%v on %s = %v, want %v", c.sq, c.by, c.fen, got, c.want)
		}
	}
}

func TestInCheck(t *testing.T) {
	b, _ := FromFEN("rnb1kbnr/pppp1ppp/8/4p3/6Pq/5P2/PPPPP2P/RNBQKBNR w KQkq - 1 3")
	if !b.InCheck(White) {
		t.Fatal("white should be in check (Qh4)")
	}
	if b.InCheck(Black) {
		t.Fatal("black is not in check")
	}
}
