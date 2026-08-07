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
