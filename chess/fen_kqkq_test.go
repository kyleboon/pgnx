package chess

import "testing"

func TestKQkqResolvesOutermostRooks(t *testing.T) {
	b, err := FromFEN("nqbrkbrn/pppppppp/8/8/8/8/PPPPPPPP/NQBRKBRN w KQkq - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	want := CastlingRights{
		WhiteKingside: true, WhiteKRookFile: 6,
		WhiteQueenside: true, WhiteQRookFile: 3,
		BlackKingside: true, BlackKRookFile: 6,
		BlackQueenside: true, BlackQRookFile: 3,
	}
	if b.Castling != want {
		t.Errorf("Castling = %+v, want %+v", b.Castling, want)
	}
	// No rook on the back rank: no right is granted (pgn-extract behaviour).
	b, err = FromFEN("4k3/8/8/8/8/8/8/4K2R w Kkq - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	want = CastlingRights{WhiteKingside: true, WhiteKRookFile: 7}
	if b.Castling != want {
		t.Errorf("Castling = %+v, want %+v", b.Castling, want)
	}
}
