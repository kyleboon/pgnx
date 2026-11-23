package chess_test

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kyleboon/pgnx/chess"
	"github.com/kyleboon/pgnx/chess/internal/mailbox"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// differentialCorpora are the PGN files whose every position, main line
// and variations alike, the bitboard engine is compared with the mailbox
// oracle on (ADR 0005): the replay benchmark corpus and the golden
// conformance corpus vendored from the C tool's test suite.
func differentialCorpora(t *testing.T) []string {
	t.Helper()
	files := []string{filepath.Join("..", "replay", "testdata", "bench.pgn")}
	golden, err := filepath.Glob(filepath.Join("..", "cmd", "pgnx", "testdata", "golden", "infiles", "*.pgn"))
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, golden...)
	files = append(files, filepath.Join("..", "cmd", "pgnx", "testdata", "golden", "eco.pgn"))
	return files
}

// TestBitboardAgreesWithMailboxOracle replays every game of the
// differential corpora and, at every position reached, compares the
// chess.Board with the mailbox oracle built from the same pieces: FEN,
// the king squares, check, attacks on every square by both colours, the
// legal-move set, the targeted candidates for every piece type and
// destination, the castling moves, the mate and material predicates, and
// the placement key and weak hash against a scan of the oracle's squares.
func TestBitboardAgreesWithMailboxOracle(t *testing.T) {
	positions := 0
	for _, path := range differentialCorpora(t) {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		sc := pgn.NewScanner(f, pgn.ParserOptions{})
		for n := 1; sc.Next(); n++ {
			g := sc.Game()
			r, _ := replay.Game(g, replay.Options{AllowNullMoves: true})
			if r == nil {
				continue
			}
			positions += compareLine(t, path, n, r.StartBoard(), r)
		}
		if err := sc.Err(); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		f.Close()
	}
	t.Logf("compared %d positions", positions)
	if positions < 10000 {
		t.Errorf("compared only %d positions; the corpora should give tens of thousands", positions)
	}
}

// compareLine walks moves from b, comparing both engines at every
// position and recursing into each move's variations, and returns the
// number of positions compared. It stops the test at the first mismatch.
func compareLine(t *testing.T, path string, game int, b *chess.Board, r *replay.Result) int {
	t.Helper()
	ob := mailbox.FromBoard(b)
	moves := r.Moves
	n := 0
	for i := 0; ; i++ {
		comparePosition(t, path, game, i, b, ob)
		n++
		if i == len(moves) {
			return n
		}
		for _, v := range r.Variations(i) {
			if v != nil {
				n += compareLine(t, path, game, b.Copy(), v)
			}
		}
		b.Apply(moves[i].Move)
		ob.Apply(moves[i].Move)
	}
}

func comparePosition(t *testing.T, path string, game, ply int, b *chess.Board, ob *mailbox.Board) {
	t.Helper()
	fail := func(format string, args ...any) {
		t.Helper()
		t.Fatalf("%s game %d ply %d (%s): "+format, append([]any{path, game, ply, ob.FEN()}, args...)...)
	}
	if got, want := b.FEN(), ob.FEN(); got != want {
		fail("FEN = %q, want %q", got, want)
	}
	if got, want := b.PlacementKey(), oraclePlacementKey(ob); got != want {
		fail("PlacementKey = %#x, want %#x", got, want)
	}
	if got, want := b.WeakHash(), oracleWeakHash(ob); got != want {
		fail("WeakHash = %#x, want %#x", got, want)
	}
	for _, c := range []chess.Color{chess.White, chess.Black} {
		if got, want := b.KingSquare(c), ob.KingSquare(c); got != want {
			fail("KingSquare(%v) = %v, want %v", c, got, want)
		}
		if got, want := b.InCheck(c), ob.InCheck(c); got != want {
			fail("InCheck(%v) = %v, want %v", c, got, want)
		}
		for _, sq := range allSquares() {
			if got, want := b.IsAttacked(sq, c), ob.IsAttacked(sq, c); got != want {
				fail("IsAttacked(%v, %v) = %v, want %v", sq, c, got, want)
			}
		}
	}
	if got, want := sortedMoves(b.LegalMoves()), sortedMoves(ob.LegalMoves()); !slices.Equal(got, want) {
		fail("LegalMoves = %v, want %v", got, want)
	}
	for _, pt := range []chess.PieceType{chess.Pawn, chess.Knight, chess.Bishop, chess.Rook, chess.Queen, chess.King} {
		for _, to := range allSquares() {
			got := sortedMoves(b.AppendLegalMovesTo(pt, to, nil))
			want := sortedMoves(ob.AppendLegalMovesTo(pt, to, nil))
			if !slices.Equal(got, want) {
				fail("AppendLegalMovesTo(%v, %v) = %v, want %v", pt, to, got, want)
			}
		}
	}
	for _, side := range []chess.CastleSide{chess.Kingside, chess.Queenside} {
		gm, gok := b.CastleMove(side)
		wm, wok := ob.CastleMove(side)
		if gm != wm || gok != wok {
			fail("CastleMove(%v) = %v, %v, want %v, %v", side, gm, gok, wm, wok)
		}
	}
	if got, want := b.HasLegalMove(), ob.HasLegalMove(); got != want {
		fail("HasLegalMove = %v, want %v", got, want)
	}
	if got, want := b.IsCheckmate(), ob.IsCheckmate(); got != want {
		fail("IsCheckmate = %v, want %v", got, want)
	}
	if got, want := b.IsStalemate(), ob.IsStalemate(); got != want {
		fail("IsStalemate = %v, want %v", got, want)
	}
	if got, want := b.InsufficientMaterial(), ob.InsufficientMaterial(); got != want {
		fail("InsufficientMaterial = %v, want %v", got, want)
	}
}

func allSquares() []chess.Square {
	sqs := make([]chess.Square, 0, 64)
	for rank := 0; rank < 8; rank++ {
		for file := 0; file < 8; file++ {
			sqs = append(sqs, chess.Square{File: file, Rank: rank})
		}
	}
	return sqs
}

func sortedMoves(moves []chess.Move) []chess.Move {
	s := slices.Clone(moves)
	slices.SortFunc(s, func(a, b chess.Move) int {
		return cmp.Or(
			cmp.Compare(a.From.Rank, b.From.Rank), cmp.Compare(a.From.File, b.From.File),
			cmp.Compare(a.To.Rank, b.To.Rank), cmp.Compare(a.To.File, b.To.File),
			cmp.Compare(a.Promotion, b.Promotion), cmp.Compare(a.Castle, b.Castle),
		)
	})
	return s
}

// oraclePlacementKey is the Polyglot placement hash of the oracle's
// squares by definition: the XOR over occupied squares of the random for
// (piece kind, square), kinds numbered black pawn 0, white pawn 1, black
// knight 2 and so on to white king 11, squares 8*rank+file.
func oraclePlacementKey(ob *mailbox.Board) uint64 {
	var h uint64
	for _, sq := range allSquares() {
		p := ob.PieceAt(sq)
		if p.Type == chess.NoPiece {
			continue
		}
		kind := 2 * (int(p.Type) - 1)
		if p.Color == chess.White {
			kind++
		}
		h ^= chess.PolyglotRandom[64*kind+8*sq.Rank+sq.File]
	}
	return h
}

// oracleWeakHash is pgn-extract's weak_hash_value of the oracle's squares
// by definition: the XOR of HashTab[file][rank][piece][colour] over
// occupied squares, colour 1 for White.
func oracleWeakHash(ob *mailbox.Board) uint64 {
	var h uint64
	for _, sq := range allSquares() {
		p := ob.PieceAt(sq)
		if p.Type == chess.NoPiece {
			continue
		}
		colour := 0
		if p.Color == chess.White {
			colour = 1
		}
		h ^= chess.WeakTable[sq.File][sq.Rank][p.Type-1][colour]
	}
	return h
}
