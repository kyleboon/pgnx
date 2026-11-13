package chess

import "testing"

// castlingFENs are positions whose castling legality turns on each rule
// castleMove applies: rights, a clear path, a king in check, an attacked
// crossing or destination square, and Chess960 rook files (Shredder-FEN)
// where the king's and rook's paths overlap or coincide.
var castlingFENs = []struct{ name, fen string }{
	{"both sides open", "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1"},
	{"black both sides open", "r3k2r/8/8/8/8/8/8/R3K2R b KQkq - 0 1"},
	{"no rights", "r3k2r/8/8/8/8/8/8/R3K2R w - - 0 1"},
	{"kingside path blocked", "r3k2r/8/8/8/8/8/8/R3KB1R w KQkq - 0 1"},
	{"queenside path blocked by knight", "r3k2r/8/8/8/8/8/8/RN2K2R w KQkq - 0 1"},
	{"king in check", "r3k2r/8/8/8/8/4r3/8/R3K2R w KQkq - 0 1"},
	{"f1 attacked", "r3k2r/8/8/8/8/5r2/8/R3K2R w KQkq - 0 1"},
	{"g1 attacked", "r3k2r/8/8/8/8/6r1/8/R3K2R w KQkq - 0 1"},
	{"b1 attacked only", "r3k2r/8/8/8/8/1r6/8/R3K2R w KQkq - 0 1"},
	{"d1 attacked", "r3k2r/8/8/8/8/3r4/8/R3K2R w KQkq - 0 1"},
	{"black c8 attacked", "r3k2r/8/2R5/8/8/8/8/4K3 b kq - 0 1"},
	{"960 king b1 rook a1 queenside", "1r2kr2/8/8/8/8/8/8/RK3R2 w FAfb - 0 1"},
	{"960 king b1 rook g1 kingside", "1r4k1/8/8/8/8/8/8/1K4R1 w G - 0 1"},
	{"960 king f1 rooks e1 h1", "4k2r/8/8/8/8/8/8/4RK1R w HEh - 0 1"},
	{"960 queenside rook c1 king d1", "2r1k3/8/8/8/8/8/8/2RK4 w Cc - 0 1"},
	{"960 rook shields king but not its destination", "6k1/8/8/8/8/8/8/K5Rr w G - 0 1"},
	{"kiwipete", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1"},
	{"kiwipete black", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R b KQkq - 0 1"},
}

// TestCastleMoveAgreesWithLegalMoves pins CastleMove to the castling
// entry of the full legal-move list, side by side, position by position.
func TestCastleMoveAgreesWithLegalMoves(t *testing.T) {
	for _, c := range append(castlingFENs, generatorFENs...) {
		b, err := FromFEN(c.fen)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		legal := b.LegalMoves()
		for _, side := range []CastleSide{Kingside, Queenside} {
			var want Move
			found := false
			for _, m := range legal {
				if m.Castle == side {
					if found {
						t.Fatalf("%s: two %v castling moves in LegalMoves", c.name, side)
					}
					want, found = m, true
				}
			}
			got, ok := b.CastleMove(side)
			if ok != found || got != want {
				t.Errorf("%s: CastleMove(%v) = %v, %v; LegalMoves has %v, %v", c.name, side, got, ok, want, found)
			}
		}
	}
}

func TestCastleMoveAllocatesNothing(t *testing.T) {
	for _, c := range castlingFENs {
		b := mustFromFEN(c.fen)
		for _, side := range []CastleSide{Kingside, Queenside} {
			if got := testing.AllocsPerRun(100, func() { b.CastleMove(side) }); got != 0 {
				t.Errorf("%s: CastleMove(%v) allocated %v times/op, want 0", c.name, side, got)
			}
		}
	}
}
