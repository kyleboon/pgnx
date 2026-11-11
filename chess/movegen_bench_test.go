package chess

import "testing"

// BenchmarkLegalMoves measures full legal-move generation, the cost that
// dominates replay (ADR 0003): the starting position and the "Kiwipete"
// perft position, which exercises castling, en passant, pins and
// promotions.
func BenchmarkLegalMoves(b *testing.B) {
	for _, c := range []struct{ name, fen string }{
		{"start", startPositionFEN},
		{"kiwipete", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1"},
	} {
		b.Run(c.name, func(b *testing.B) {
			board := mustFromFEN(c.fen)
			b.ReportAllocs()
			for b.Loop() {
				board.LegalMoves()
			}
		})
	}
}
