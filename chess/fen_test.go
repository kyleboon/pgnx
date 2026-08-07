package chess

import "testing"

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

func TestFromFENRejectsBad(t *testing.T) {
	for _, fen := range []string{"", "x", "8/8/8/8/8/8/8/8", "9/8/8/8/8/8/8/8 w - - 0 1"} {
		if _, err := FromFEN(fen); err == nil {
			t.Errorf("FromFEN(%q): want error", fen)
		}
	}
}
