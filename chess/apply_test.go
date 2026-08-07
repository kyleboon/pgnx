package chess

import "testing"

func apply(t *testing.T, fen string, m Move) *Board {
	t.Helper()
	b, err := FromFEN(fen)
	if err != nil {
		t.Fatal(err)
	}
	b.Apply(m)
	return b
}

func TestApplyBasics(t *testing.T) {
	cases := []struct {
		name, fen string
		m         Move
		want      string
	}{
		{"double push sets EP",
			"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1",
			Move{From: MustSquare("e2"), To: MustSquare("e4")},
			"rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1"},
		{"capture resets halfmove clock",
			"rnbqkbnr/ppp1pppp/8/3p4/4P3/8/PPPP1PPP/RNBQKBNR w KQkq - 4 2",
			Move{From: MustSquare("e4"), To: MustSquare("d5")},
			"rnbqkbnr/ppp1pppp/8/3P4/8/8/PPPP1PPP/RNBQKBNR b KQkq - 0 2"},
		{"en passant removes captured pawn",
			"rnbqkbnr/ppp1p1pp/8/3pPp2/8/8/PPPP1PPP/RNBQKBNR w KQkq f6 0 3",
			Move{From: MustSquare("e5"), To: MustSquare("f6")},
			// Capturing pawn lands on f6 (the move's To square); the
			// brief's original "want" placed it on f3, an off-by-rank
			// transcription error inconsistent with the move given.
			"rnbqkbnr/ppp1p1pp/5P2/3p4/8/8/PPPP1PPP/RNBQKBNR b KQkq - 0 3"},
		{"promotion",
			"8/P7/8/8/8/8/7k/K7 w - - 0 1",
			Move{From: MustSquare("a7"), To: MustSquare("a8"), Promotion: Queen},
			"Q7/8/8/8/8/8/7k/K7 b - - 0 1"},
		{"white O-O moves rook, clears rights",
			"r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1",
			Move{From: MustSquare("e1"), To: MustSquare("g1"), Castle: Kingside},
			"r3k2r/8/8/8/8/8/8/R4RK1 b kq - 1 1"},
		{"black O-O-O",
			"r3k2r/8/8/8/8/8/8/R4RK1 b kq - 1 1",
			Move{From: MustSquare("e8"), To: MustSquare("c8"), Castle: Queenside},
			"2kr3r/8/8/8/8/8/8/R4RK1 w - - 2 2"},
		{"rook move clears one side's rights",
			"r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1",
			Move{From: MustSquare("h1"), To: MustSquare("h5")},
			// Rook lands on h5 per the move's To square; the brief's
			// original "want" placed it on h4, an off-by-rank
			// transcription error inconsistent with the move given.
			"r3k2r/8/8/7R/8/8/8/R3K3 b Qkq - 1 1"},
		{"capturing a rook on its home square clears opponent rights",
			"r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1",
			Move{From: MustSquare("a1"), To: MustSquare("a8")},
			"R3k2r/8/8/8/8/8/8/4K2R b Kk - 0 1"},
	}
	for _, c := range cases {
		if got := apply(t, c.fen, c.m).FEN(); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}

func TestApplyUCI(t *testing.T) {
	m := Move{From: MustSquare("e7"), To: MustSquare("e8"), Promotion: Queen}
	if m.UCI() != "e7e8q" {
		t.Errorf("UCI() = %q", m.UCI())
	}
}
