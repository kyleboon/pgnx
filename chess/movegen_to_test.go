package chess

import (
	"reflect"
	"sort"
	"testing"
)

// targetedFENs are the positions the targeted generator is checked on:
// the generator positions (castling, en passant, pins, checks,
// promotions) and the perft positions, with both sides to move among
// them, plus positions where a pawn's push, double push, capture and en
// passant capture to one square compete and where pins and checks cut
// candidates down.
var targetedFENs = []string{
	startPositionFEN,
	"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1",
	"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R b KQkq - 0 1",
	"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1",
	"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 b - - 0 1",
	"r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1",
	"r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 b kq - 0 1",
	"rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8",
	"rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R b KQ - 1 8",
	"rnb1kbnr/pppp1ppp/8/4p3/6Pq/5P2/PPPPP2P/RNBQKBNR w KQkq - 1 3",
	// Both white pawns may capture on d5 en passant, the e-pawn is pinned
	// against its king by the rook on e8.
	"4r3/8/8/2PpP3/8/8/8/4K2k w - d6 0 1",
	// Both white pawns may capture on d6 en passant.
	"8/8/8/2PpP3/8/8/8/4K2k w - d6 0 1",
	// After "1. e4 --" the en passant square e3 is still set with White
	// to move: the d2 and f2 pawns must not capture onto it.
	"rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR w KQkq e3 0 1",
	// Black pawns compete for c3: a push from c4, captures from b4 and
	// d4, and a double push from c7 is blocked by the pawn on c4.
	"4k3/2p5/8/8/1ppp4/8/8/4K3 b - - 0 1",
	// A pawn may promote by push or by capture onto two squares; the
	// white king is in check from a rook, so only interpositions and
	// captures of the checker are legal.
	"1nr1k3/1P6/8/8/8/8/8/r3K3 w - - 0 1",
	// Three queens reach e5, one of them pinned; two knights and two
	// rooks reach d5 and d1.
	"3rk3/8/8/3r4/8/1N1Q1Q2/8/3K1N1Q w - - 0 1",
}

func allPieceTypes() []PieceType {
	return []PieceType{Pawn, Knight, Bishop, Rook, Queen, King}
}

func sortMoves(moves []Move) {
	sort.Slice(moves, func(i, j int) bool {
		a, b := moves[i], moves[j]
		if a.From != b.From {
			return a.From.File < b.From.File || (a.From.File == b.From.File && a.From.Rank < b.From.Rank)
		}
		return a.Promotion < b.Promotion
	})
}

// TestAppendLegalMovesToAgreesWithLegalMoves checks the targeted
// generator, for every piece type and destination square, against the
// full legal-move list filtered to the non-castling moves of that piece
// type to that square.
func TestAppendLegalMovesToAgreesWithLegalMoves(t *testing.T) {
	for _, fen := range targetedFENs {
		b := mustFromFEN(fen)
		legal := b.LegalMoves()
		for _, pt := range allPieceTypes() {
			for file := 0; file < 8; file++ {
				for rank := 0; rank < 8; rank++ {
					to := Square{File: file, Rank: rank}
					var want []Move
					for _, m := range legal {
						if m.Castle == NoCastle && m.To == to && b.PieceAt(m.From).Type == pt {
							want = append(want, m)
						}
					}
					got := b.AppendLegalMovesTo(pt, to, nil)
					sortMoves(want)
					sortMoves(got)
					if len(want) == 0 && len(got) == 0 {
						continue
					}
					if !reflect.DeepEqual(got, want) {
						t.Errorf("%s: AppendLegalMovesTo(%v, %s) = %v, want %v", fen, pt, to, got, want)
					}
				}
			}
		}
	}
}
