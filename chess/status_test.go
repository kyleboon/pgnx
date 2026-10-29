package chess

import "testing"

func TestCheckmateStalemate(t *testing.T) {
	fool, _ := FromFEN("rnb1kbnr/pppp1ppp/8/4p3/6Pq/5P2/PPPPP2P/RNBQKBNR w KQkq - 1 3")
	if !fool.IsCheckmate() || fool.IsStalemate() {
		t.Error("fool's mate: want checkmate")
	}
	stale, _ := FromFEN("7k/5Q2/6K1/8/8/8/8/8 b - - 0 1")
	if !stale.IsStalemate() || stale.IsCheckmate() {
		t.Error("want stalemate")
	}
	open, _ := FromFEN("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1")
	if open.IsCheckmate() || open.IsStalemate() {
		t.Error("start position is neither")
	}
}

func TestInsufficientMaterial(t *testing.T) {
	cases := []struct {
		fen  string
		want bool
	}{
		{"8/8/8/8/8/8/8/K6k w - - 0 1", true},     // K v K
		{"8/8/8/8/8/8/8/KB5k w - - 0 1", true},    // KB v K
		{"8/8/8/8/8/8/8/KN5k w - - 0 1", true},    // KN v K
		{"8/8/8/8/8/8/8/KNN4k w - - 0 1", true},   // KNN v K: C quirk
		{"8/8/8/8/8/8/8/KB4bk w - - 0 1", false},  // KB v KB opposite colours (b1 dark, g1 light)
		{"8/8/8/8/8/8/8/KB3b1k w - - 0 1", true},  // KB v KB same colour (b1, f1 both dark)
		{"8/8/8/8/8/8/8/KBN4k w - - 0 1", false},  // KBN v K
		{"8/8/8/8/8/8/8/KNN3nk w - - 0 1", false}, // KNN v KN
		{"8/8/8/8/8/8/8/KP5k w - - 0 1", false},   // KP v K
		{"8/8/8/8/8/8/8/KR5k w - - 0 1", false},   // KR v K
		{"8/8/8/8/8/8/8/KQ5k w - - 0 1", false},   // KQ v K
		{"8/8/8/8/8/8/8/KN4nk w - - 0 1", true},   // KN v KN
	}
	for _, c := range cases {
		b, err := FromFEN(c.fen)
		if err != nil {
			t.Fatalf("%s: %v", c.fen, err)
		}
		if got := b.InsufficientMaterial(); got != c.want {
			t.Errorf("%s: InsufficientMaterial = %v, want %v", c.fen, got, c.want)
		}
	}
}
