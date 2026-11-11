package chess

import "testing"

// BenchmarkLegalMoves measures full legal-move generation, the cost that
// dominates replay (ADR 0003), on the starting position and the
// "Kiwipete" perft position, which exercises castling, en passant, pins
// and promotions. LegalMoves allocates its result per call;
// AppendLegalMoves generates into a buffer the benchmark keeps across
// iterations, the way a replay does, and should report zero allocs/op.
func BenchmarkLegalMoves(b *testing.B) {
	for _, c := range generatorFENs[:2] {
		b.Run(c.name, func(b *testing.B) {
			board := mustFromFEN(c.fen)
			b.ReportAllocs()
			for b.Loop() {
				benchSink = board.LegalMoves()
			}
		})
	}
}

// benchSink keeps LegalMoves' result alive so the compiler cannot place it
// on the stack and hide the allocation a real caller pays.
var benchSink []Move

func BenchmarkAppendLegalMoves(b *testing.B) {
	for _, c := range generatorFENs[:2] {
		b.Run(c.name, func(b *testing.B) {
			board := mustFromFEN(c.fen)
			buf := board.AppendLegalMoves(nil)
			b.ReportAllocs()
			for b.Loop() {
				buf = board.AppendLegalMoves(buf[:0])
			}
		})
	}
}

// BenchmarkIsStalemate measures the no-check fast path of the mate checks,
// which replay runs after every ply.
func BenchmarkIsStalemate(b *testing.B) {
	board := mustFromFEN(generatorFENs[1].fen)
	b.ReportAllocs()
	for b.Loop() {
		board.IsStalemate()
	}
}
