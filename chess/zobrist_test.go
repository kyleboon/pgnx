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
