package chess

import (
	"fmt"
	"testing"
)

// scanPlacementKey is the reference placement key: a full scan of the board
// XORing the Polyglot piece-square term of every occupied square. The Board
// maintains its key incrementally in setPiece; this scan is the independent
// definition the incremental key must equal in every reachable position.
func scanPlacementKey(b *Board) uint64 {
	var h uint64
	for file := 0; file < 8; file++ {
		for rank := 0; rank < 8; rank++ {
			p := b.squares[file][rank]
			if p.Type == NoPiece {
				continue
			}
			h ^= polyglotRandom[64*polyglotKind(p)+8*rank+file]
		}
	}
	return h
}

// checkKeys asserts b's incremental placement key and the Zobrist hash
// derived from it agree with the full-scan reference.
func checkKeys(t *testing.T, b *Board, context string) {
	t.Helper()
	want := scanPlacementKey(b)
	if got := b.PlacementKey(); got != want {
		t.Fatalf("%s: PlacementKey = %#x, full scan %#x (FEN %s)", context, got, want, b.FEN())
	}
	// Zobrist is the placement key plus castling, en passant and side terms,
	// so with the placement key right it must agree with a board rebuilt
	// from the FEN, whose key is seeded from scratch.
	rebuilt, err := FromFEN(b.FEN())
	if err != nil {
		t.Fatalf("%s: %v", context, err)
	}
	if got, want := b.Zobrist(), rebuilt.Zobrist(); got != want {
		t.Fatalf("%s: Zobrist = %#x, rebuilt from FEN %#x (FEN %s)", context, got, want, b.FEN())
	}
}

// walkKeys applies every legal move from b to the given depth, checking the
// keys after each application.
func walkKeys(t *testing.T, b *Board, depth int) {
	t.Helper()
	if depth == 0 {
		return
	}
	for _, m := range b.LegalMoves() {
		c := b.Copy()
		c.Apply(m)
		checkKeys(t, c, fmt.Sprintf("after %v", m))
		walkKeys(t, c, depth-1)
	}
}

// TestPlacementKeyIncremental cross-checks the incremental placement key
// against the full-scan definition after every move of the perft suite
// positions (castling, en passant, promotions, Chess960 rook files), and
// across a null move, which leaves the placement untouched.
func TestPlacementKeyIncremental(t *testing.T) {
	fens := []string{
		"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1",
		"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1",
		"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1",
		"r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1",
		"rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8",
		// Chess960: king and rook start/end squares overlap when castling.
		"1r2k1r1/pppppppp/8/8/8/8/PPPPPPPP/1R2K1R1 w GBgb - 0 1",
	}
	for _, fen := range fens {
		b, err := FromFEN(fen)
		if err != nil {
			t.Fatal(err)
		}
		checkKeys(t, b, "FromFEN "+fen)
		walkKeys(t, b, 3)

		c := b.Copy()
		c.Apply(NullMove)
		if got, want := c.PlacementKey(), b.PlacementKey(); got != want {
			t.Fatalf("null move changed the placement key of %s: %#x != %#x", fen, got, want)
		}
		checkKeys(t, c, "null move from "+fen)
	}
}
