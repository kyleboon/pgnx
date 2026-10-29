// chess/zobrist_test.go — key values published in the Polyglot book format spec
package chess

import "testing"

func TestPolyglotHashes(t *testing.T) {
	cases := []struct {
		fen  string
		want uint64
	}{
		{"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1", 0x463b96181691fc9c},
		// after e2e4: EP square e3 exists but no black pawn can capture -> NOT hashed
		{"rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1", 0x823c9b50fd114196},
		{"rnbqkbnr/ppp1pppp/8/3p4/4P3/8/PPPP1PPP/RNBQKBNR w KQkq d6 0 2", 0x0756b94461c50fb0},
		{"rnbqkbnr/ppp1pppp/8/3pP3/8/8/PPPP1PPP/RNBQKBNR b KQkq - 0 2", 0x662fafb965db29d4},
		// EP square f6 with white pawn on e5 able to capture -> IS hashed
		{"rnbqkbnr/ppp1p1pp/8/3pPp2/8/8/PPPP1PPP/RNBQKBNR w KQkq f6 0 3", 0x22a48b5a8e47ff78},
	}
	for _, c := range cases {
		b, err := FromFEN(c.fen)
		if err != nil {
			t.Fatal(err)
		}
		if got := b.Zobrist(); got != c.want {
			t.Errorf("Zobrist(%s) = %#x, want %#x", c.fen, got, c.want)
		}
	}
}

func TestPlacementKey(t *testing.T) {
	mustFEN := func(fen string) *Board {
		t.Helper()
		b, err := FromFEN(fen)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}

	// Positions that differ only in side to move, castling rights, or the
	// en passant square share a placement key but not a Zobrist hash.
	base := mustFEN("rnbqkbnr/ppp1p1pp/8/3pPp2/8/8/PPPP1PPP/RNBQKBNR w KQkq f6 0 3")
	variants := []string{
		"rnbqkbnr/ppp1p1pp/8/3pPp2/8/8/PPPP1PPP/RNBQKBNR b KQkq f6 0 3", // side to move
		"rnbqkbnr/ppp1p1pp/8/3pPp2/8/8/PPPP1PPP/RNBQKBNR w Kq f6 0 3",   // castling
		"rnbqkbnr/ppp1p1pp/8/3pPp2/8/8/PPPP1PPP/RNBQKBNR w KQkq - 0 3",  // ep (capturable in base)
		"rnbqkbnr/ppp1p1pp/8/3pPp2/8/8/PPPP1PPP/RNBQKBNR w - - 7 40",    // clocks too
	}
	for _, fen := range variants {
		v := mustFEN(fen)
		if v.PlacementKey() != base.PlacementKey() {
			t.Errorf("PlacementKey differs for %s", fen)
		}
		if v.Zobrist() == base.Zobrist() {
			t.Errorf("Zobrist unexpectedly equal for %s", fen)
		}
	}

	// A single move changes the placement key.
	start := mustFEN("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1")
	afterE4 := mustFEN("rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1")
	if start.PlacementKey() == afterE4.PlacementKey() {
		t.Error("PlacementKey unchanged after 1.e4")
	}

	// The key is the piece-square part of the Polyglot hash: for the
	// start position the Zobrist hash differs from it only by the four
	// castling terms and the turn term.
	want := start.Zobrist()
	for i := polyglotCastleOffset; i < polyglotCastleOffset+4; i++ {
		want ^= polyglotRandom[i]
	}
	want ^= polyglotRandom[polyglotTurnOffset]
	if got := start.PlacementKey(); got != want {
		t.Errorf("PlacementKey(start) = %#x, want %#x", got, want)
	}
}
