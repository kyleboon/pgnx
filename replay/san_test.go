package replay

import (
	"testing"

	"github.com/kyleboon/pgnx/chess"
)

func board(t *testing.T, fen string) *chess.Board {
	t.Helper()
	b, err := chess.FromFEN(fen)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDecodeSAN(t *testing.T) {
	start := "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"
	cases := []struct{ fen, san, uci string }{
		{start, "e4", "e2e4"},
		{start, "Nf3", "g1f3"},
		{start, "Sf3", "g1f3"},  // German knight
		{start, "e2e4", "e2e4"}, // long algebraic accepted
		{start, "e2-e4", "e2e4"},
		{"rnbqkbnr/ppp1pppp/8/3p4/4P3/8/PPPP1PPP/RNBQKBNR w KQkq - 0 2", "exd5", "e4d5"},
		{"rnbqkbnr/ppp1pppp/8/3p4/4P3/8/PPPP1PPP/RNBQKBNR w KQkq - 0 2", "e:d5", "e4d5"},
		// two knights can reach d2: disambiguation required and honored
		{"rnbqkb1r/pppppppp/5n2/8/8/5N2/PPP1PPPP/RNBQKB1R w KQkq - 0 1", "Nbd2", "b1d2"},
		{"rnbqkb1r/pppppppp/5n2/8/8/5N2/PPP1PPPP/RNBQKB1R w KQkq - 0 1", "Nfd2", "f3d2"},
		{"8/P6k/8/8/8/8/8/K7 w - - 0 1", "a8=Q", "a7a8q"},
		{"8/P6k/8/8/8/8/8/K7 w - - 0 1", "a8Q", "a7a8q"},
		{"r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1", "O-O", "e1g1"},
		{"r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1", "0-0-0", "e1c1"},
		{"rnbqkbnr/pppp1ppp/8/4p3/6P1/5P2/PPPPP2P/RNBQKBNR b KQkq g3 0 2", "Qh4#", "d8h4"},
		// lowercase long-algebraic promotion letters (C oracle: is_piece is
		// case-insensitive for K/Q/D/R/T/N/S, and 'b' is specially allowed as
		// a trailing promotion letter per decode.c's "allow a trailing 'b' as
		// a Bishop promotion").
		{"7k/3P4/7K/8/8/8/8/8 w - - 0 1", "d7d8r", "d7d8r"},
		{"7k/3P4/7K/8/8/8/8/8 w - - 0 1", "d7d8n", "d7d8n"},
		{"7k/3P4/7K/8/8/8/8/8 w - - 0 1", "d7d8b", "d7d8b"},
		{"7k/3P4/7K/8/8/8/8/8 w - - 0 1", "d7d8q", "d7d8q"},
		// bare (no '=') SAN promotion also accepts lowercase.
		{"7k/3P4/7K/8/8/8/8/8 w - - 0 1", "d8q", "d7d8q"},
		// bare long-algebraic PIECE move: full from-square hint (file+rank)
		// disambiguates by square alone, regardless of piece type, so a
		// leading square that happens to look like "no piece letter" must
		// not be forced to mean "pawn move" when no pawn is present there.
		{"rnbqkbnr/pppppppp/8/8/8/1P6/P1PPPPPP/RNBQKBNR w KQkq - 0 1", "c1b2", "c1b2"},
		// bare long-algebraic castling: king's own from/to squares happen
		// to equal the castle move's, and the C oracle accepts this form
		// (not just O-O) - see test-o.txt/test-a.txt's "e1g1"/"e8g8".
		{"r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1", "e1g1", "e1g1"},
		{"r3k2r/8/8/8/8/8/8/R3K2R b KQkq - 0 1", "e8g8", "e8g8"},
		{"r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1", "e1c1", "e1c1"},
	}
	for _, c := range cases {
		m, err := decodeSAN(c.san, board(t, c.fen))
		if err != nil {
			t.Errorf("decodeSAN(%q): %v", c.san, err)
			continue
		}
		if m.UCI() != c.uci {
			t.Errorf("decodeSAN(%q) = %s, want %s", c.san, m.UCI(), c.uci)
		}
	}
}

func TestDecodeSANErrors(t *testing.T) {
	start := "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"
	for _, san := range []string{"e5", "Nd2xx", "O-O", "Qh4", ""} {
		if _, err := decodeSAN(san, board(t, start)); err == nil {
			t.Errorf("decodeSAN(%q): want error", san)
		}
	}
}

func TestSanFor(t *testing.T) {
	// both knights reach d2 -> minimal disambiguation is by file
	b := board(t, "rnbqkb1r/pppppppp/5n2/8/8/5N2/PPP1PPPP/RNBQKB1R w KQkq - 0 1")
	m, _ := decodeSAN("Nbd2", b)
	if got := sanFor(m, b); got != "Nbd2" {
		t.Errorf("sanFor = %q, want Nbd2", got)
	}
	b2 := board(t, "rnbqkbnr/ppp1pppp/8/3p4/4P3/8/PPPP1PPP/RNBQKBNR w KQkq - 0 2")
	m2, _ := decodeSAN("exd5", b2)
	if got := sanFor(m2, b2); got != "exd5" {
		t.Errorf("sanFor = %q, want exd5", got)
	}
}
