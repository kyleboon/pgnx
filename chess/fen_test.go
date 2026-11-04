package chess

import (
	"strings"
	"testing"
)

const startFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"

func TestNewBoardFEN(t *testing.T) {
	if got := NewBoard().FEN(); got != startFEN {
		t.Errorf("NewBoard().FEN() = %q", got)
	}
}

func TestFENRoundTrip(t *testing.T) {
	fens := []string{
		startFEN,
		"rnbqkbnr/pppp1ppp/8/4p3/8/1P6/P1PPPPPP/RNBQKBNR w KQkq e6 0 2",
		"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1",
		"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R b KQkq - 3 2",
	}
	for _, fen := range fens {
		b, err := FromFEN(fen)
		if err != nil {
			t.Fatalf("FromFEN(%q): %v", fen, err)
		}
		if got := b.FEN(); got != fen {
			t.Errorf("round trip\n in: %s\nout: %s", fen, got)
		}
	}
}

func TestFromFENDetails(t *testing.T) {
	b, err := FromFEN("rnbqkbnr/pppp1ppp/8/4p3/8/1P6/P1PPPPPP/RNBQKBNR w KQkq e6 0 2")
	if err != nil {
		t.Fatal(err)
	}
	if b.SideToMove != White || !b.HasEP || b.EPSquare != MustSquare("e6") ||
		b.FullmoveNumber != 2 || b.PieceAt(MustSquare("e5")) != (Piece{Pawn, Black}) {
		t.Fatalf("parsed state wrong: %+v", b)
	}
	if b.KingSquare(Black) != MustSquare("e8") {
		t.Error("king cache wrong")
	}
}

func TestShredderCastlingSideFromRookVsKingFile(t *testing.T) {
	tests := []struct {
		name    string
		fen     string
		want    CastlingRights
		wantFEN string // expected FEN() output (may collapse to KQkq style)
	}{
		{
			name: "white single letter A is queenside",
			fen:  "r3k2r/8/8/8/8/8/8/R3K2R w A - 0 1",
			want: CastlingRights{WhiteQueenside: true, WhiteQRookFile: 0},
			// file 0 is the standard queenside rook file, so emits Q
			wantFEN: "r3k2r/8/8/8/8/8/8/R3K2R w Q - 0 1",
		},
		{
			name:    "white single letter H is kingside",
			fen:     "r3k2r/8/8/8/8/8/8/R3K2R w H - 0 1",
			want:    CastlingRights{WhiteKingside: true, WhiteKRookFile: 7},
			wantFEN: "r3k2r/8/8/8/8/8/8/R3K2R w K - 0 1",
		},
		{
			name: "letter order AH same as HA",
			fen:  "r3k2r/8/8/8/8/8/8/R3K2R w AH - 0 1",
			want: CastlingRights{
				WhiteKingside: true, WhiteKRookFile: 7,
				WhiteQueenside: true, WhiteQRookFile: 0,
			},
			wantFEN: "r3k2r/8/8/8/8/8/8/R3K2R w KQ - 0 1",
		},
		{
			name: "letter order HA",
			fen:  "r3k2r/8/8/8/8/8/8/R3K2R w HA - 0 1",
			want: CastlingRights{
				WhiteKingside: true, WhiteKRookFile: 7,
				WhiteQueenside: true, WhiteQRookFile: 0,
			},
			wantFEN: "r3k2r/8/8/8/8/8/8/R3K2R w KQ - 0 1",
		},
		{
			name:    "black single letter a is queenside",
			fen:     "r3k2r/8/8/8/8/8/8/R3K2R w a - 0 1",
			want:    CastlingRights{BlackQueenside: true, BlackQRookFile: 0},
			wantFEN: "r3k2r/8/8/8/8/8/8/R3K2R w q - 0 1",
		},
		{
			name: "non-standard single-sided rights round-trip exactly",
			fen:  "1r2k3/8/8/8/8/8/8/4K3 b b - 0 1",
			want: CastlingRights{BlackQueenside: true, BlackQRookFile: 1},
			// rook file 1 is non-standard, so the Shredder letter is preserved
			wantFEN: "1r2k3/8/8/8/8/8/8/4K3 b b - 0 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := FromFEN(tt.fen)
			if err != nil {
				t.Fatalf("FromFEN(%q): %v", tt.fen, err)
			}
			if b.Castling != tt.want {
				t.Errorf("castling = %+v, want %+v", b.Castling, tt.want)
			}
			if got := b.FEN(); got != tt.wantFEN {
				t.Errorf("FEN() = %q, want %q", got, tt.wantFEN)
			}
		})
	}
}

func TestShredderCastlingRejectsKingFile(t *testing.T) {
	// White king is on e1 (file 4); castling letter E names the king's file.
	if _, err := FromFEN("r3k2r/8/8/8/8/8/8/R3K2R w E - 0 1"); err == nil {
		t.Error("FromFEN with castling rook file equal to king file: want error")
	}
}

func TestFromFENRejectsBad(t *testing.T) {
	for _, fen := range []string{"", "x", "8/8/8/8/8/8/8/8", "9/8/8/8/8/8/8/8 w - - 0 1"} {
		if _, err := FromFEN(fen); err == nil {
			t.Errorf("FromFEN(%q): want error", fen)
		}
	}
}

// TestCastlingFENSpelling pins the castling field of FEN() to pgn-extract's
// build_basic_EPD_string (apply.c), checked with pgn-extract --fencomments:
// a side with both rights is written K/Q (k/q) whatever files its rooks
// start on (X-FEN: the right names the outermost rook), and a side with
// one right is written K/Q only when that rook starts on its standard
// file, otherwise as the rook's file letter (Shredder-FEN).
func TestCastlingFENSpelling(t *testing.T) {
	cases := []struct{ in, want string }{
		// Chess960 start, KQkq in: rooks on d and g, both rights each side.
		{"nqbrkbrn/pppppppp/8/8/8/8/PPPPPPPP/NQBRKBRN w KQkq - 0 1", "KQkq"},
		// The same rights given as file letters, in either order.
		{"nqbrkbrn/pppppppp/8/8/8/8/PPPPPPPP/NQBRKBRN w DGdg - 0 1", "KQkq"},
		{"nqbrkbrn/pppppppp/8/8/8/8/PPPPPPPP/NQBRKBRN w GDgd - 0 1", "KQkq"},
		// One right each: White's h-rook is standard, Black's d-rook is not.
		{"nqbrknbr/pppppppp/8/8/8/8/PPPPPPPP/NQBRKNBR w Kq - 0 1", "Kd"},
		// One non-standard right each.
		{"nqbrkbrn/pppppppp/8/8/8/8/PPPPPPPP/NQBRKBRN w Kq - 0 1", "Gd"},
		// Standard start with standard rights, however spelled.
		{"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1", "KQkq"},
		{"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w HAha - 0 1", "KQkq"},
		{"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w Kq - 0 1", "Kq"},
		// Both White rights with one non-standard file still collapse.
		{"nqbrknbr/pppppppp/8/8/8/8/PPPPPPPP/NQBRKNBR w KQ - 0 1", "KQ"},
		{"nqbrknbr/pppppppp/8/8/8/8/PPPPPPPP/NQBRKNBR w - - 0 1", "-"},
	}
	for _, c := range cases {
		b, err := FromFEN(c.in)
		if err != nil {
			t.Fatalf("FromFEN(%q): %v", c.in, err)
		}
		if got := strings.Fields(b.FEN())[2]; got != c.want {
			t.Errorf("FEN(%q) castling = %q, want %q", c.in, got, c.want)
		}
		// What FEN writes, FromFEN reads back to the same rights, mixed
		// styles ("Kd") included.
		back, err := FromFEN(b.FEN())
		if err != nil {
			t.Errorf("FromFEN(FEN(%q)) = %v", c.in, err)
		} else if back.Castling != b.Castling {
			t.Errorf("FEN(%q) round trip: %+v, want %+v", c.in, back.Castling, b.Castling)
		}
	}
}

// TestParseCastlingRejectsBad: a right given twice (in either style), a
// file letter on the king's file, and a stray character are errors.
func TestParseCastlingRejectsBad(t *testing.T) {
	for _, field := range []string{"KK", "KH", "DD", "E", "X", "Kx"} {
		fen := "nqbrkbrn/pppppppp/8/8/8/8/PPPPPPPP/NQBRKBRN w " + field + " - 0 1"
		if _, err := FromFEN(fen); err == nil {
			t.Errorf("FromFEN castling %q: want error", field)
		}
	}
}
