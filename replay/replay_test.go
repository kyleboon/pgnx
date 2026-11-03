package replay

import (
	"errors"
	"strings"
	"testing"

	"github.com/kyleboon/pgnx/chess"
	"github.com/kyleboon/pgnx/pgn"
)

func parseOne(t *testing.T, src string) *pgn.Game {
	t.Helper()
	s := pgn.NewScanner(strings.NewReader(src), pgn.ParserOptions{})
	if !s.Next() {
		t.Fatalf("no game parsed: %v", s.Err())
	}
	return s.Game()
}

func TestReplayFoolsMate(t *testing.T) {
	g := parseOne(t, "1. f3 e5 2. g4 Qh4# 0-1\n")
	r, err := Game(g, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Moves) != 4 || len(r.Positions) != 5 {
		t.Fatalf("lengths: %d moves, %d positions", len(r.Moves), len(r.Positions))
	}
	last := r.Moves[3]
	if last.SAN != "Qh4#" || !last.IsMate || !last.IsCheck {
		t.Errorf("last move: %+v", last)
	}
	if r.Positions[0].Zobrist != 0x463b96181691fc9c {
		t.Errorf("initial hash wrong")
	}
	wantFinal := "rnb1kbnr/pppp1ppp/8/4p3/6Pq/5P2/PPPPP2P/RNBQKBNR w KQkq - 1 3"
	if r.Positions[4].FEN != wantFinal {
		t.Errorf("final FEN = %s", r.Positions[4].FEN)
	}
}

func TestReplayFENTagStart(t *testing.T) {
	g := parseOne(t, "[FEN \"8/P6k/8/8/8/8/8/K7 w - - 0 1\"]\n\n1. a8=Q *\n")
	r, err := Game(g, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Moves[0].SAN != "a8=Q" {
		t.Errorf("move = %+v", r.Moves[0])
	}
}

func TestReplayVariations(t *testing.T) {
	g := parseOne(t, "1. e4 e5 (1... c5 2. Nf3) 2. Nf3 *\n")
	r, err := Game(g, Options{})
	if err != nil {
		t.Fatal(err)
	}
	vs := r.Moves[1].Variations
	if len(vs) != 1 || vs[0] == nil || len(vs[0].Moves) != 2 || vs[0].Moves[0].SAN != "c5" {
		t.Fatalf("variation replay wrong: %+v", vs)
	}
}

func TestReplayIllegalMove(t *testing.T) {
	g := parseOne(t, "1. e4 e4 *\n") // black cannot play e4
	_, err := Game(g, Options{})
	var ime *IllegalMoveError
	if !errors.As(err, &ime) {
		t.Fatalf("want IllegalMoveError, got %v", err)
	}
	if ime.Ply != 2 || ime.SAN != "e4" {
		t.Errorf("error detail: %+v", ime)
	}
}

func TestPlyCount(t *testing.T) {
	cases := []struct {
		src  string
		want int
	}{
		{"1. e4 e5 2. Nf3 *\n", 3},
		{"1. f3 e5 2. g4 Qh4# 0-1\n", 4},
		{"*\n", 0},
		{"[FEN \"8/8/8/8/8/8/k7/4K2R b K - 0 40\"]\n\n40... Kb2 *\n", 80},
		{"[FEN \"8/8/8/8/8/8/k7/4K2R w K - 0 40\"]\n\n40. O-O *\n", 79},
	}
	for _, c := range cases {
		g := parseOne(t, c.src)
		r, err := Game(g, Options{})
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		if got := r.PlyCount(); got != c.want {
			t.Errorf("%q: PlyCount = %d, want %d", c.src, got, c.want)
		}
	}
}

func TestPositionClocks(t *testing.T) {
	g := parseOne(t, "1. Nf3 Nf6 2. Ng1 *\n")
	r, err := Game(g, Options{})
	if err != nil {
		t.Fatal(err)
	}
	last := r.Positions[3]
	if last.HalfmoveClock != 3 || last.FullmoveNumber != 2 || last.SideToMove != chess.Black {
		t.Errorf("final position clocks: %+v", last)
	}
}

func TestBoardsOnDemand(t *testing.T) {
	g := parseOne(t, "1. f3 e5 2. g4 Qh4# 0-1\n")
	r, err := Game(g, Options{})
	if err != nil {
		t.Fatal(err)
	}
	start, final := r.StartBoard(), r.FinalBoard()
	if start.FEN() != r.Positions[0].FEN {
		t.Errorf("StartBoard FEN = %s", start.FEN())
	}
	if final.FEN() != r.Positions[4].FEN || !final.IsCheckmate() {
		t.Errorf("FinalBoard FEN = %s", final.FEN())
	}
	// The boards are the caller's to mutate: applying the game's typed
	// moves to StartBoard walks the positions, and a later call still
	// returns the start position.
	for i, mi := range r.Moves {
		start.Apply(mi.Move)
		if start.FEN() != r.Positions[i+1].FEN {
			t.Errorf("after move %d: %s, want %s", i, start.FEN(), r.Positions[i+1].FEN)
		}
	}
	if r.StartBoard().FEN() != r.Positions[0].FEN {
		t.Error("StartBoard was mutated through the returned board")
	}
	if (&Result{}).StartBoard() != nil || (&Result{}).FinalBoard() != nil || (*Result)(nil).FinalBoard() != nil {
		t.Error("empty result should have no boards")
	}
}

// TestBoardsOfPartialReplay pins that a replay stopped by an illegal move
// still offers the boards of its partial result: the final board is the
// position the failing move was attempted from.
func TestBoardsOfPartialReplay(t *testing.T) {
	g := parseOne(t, "1. e4 e5 2. Nf3 Nf3 *\n") // no black knight reaches f3
	r, err := Game(g, Options{})
	if err == nil {
		t.Fatal("expected replay failure")
	}
	if len(r.Positions) != 4 || r.FinalBoard().FEN() != r.Positions[3].FEN {
		t.Errorf("partial FinalBoard = %s, positions %d", r.FinalBoard().FEN(), len(r.Positions))
	}
	if r.StartBoard().FEN() != r.Positions[0].FEN {
		t.Errorf("partial StartBoard = %s", r.StartBoard().FEN())
	}
}

// TestVariationBoards pins that a variation's Result carries boards of
// its own: its start is the position before the move it branches from.
func TestVariationBoards(t *testing.T) {
	g := parseOne(t, "1. e4 e5 (1... c5 2. Nf3) 2. Nf3 *\n")
	r, err := Game(g, Options{})
	if err != nil {
		t.Fatal(err)
	}
	v := r.Moves[1].Variations[0]
	if v.StartBoard().FEN() != r.Positions[1].FEN || v.FinalBoard().FEN() != v.Positions[2].FEN {
		t.Errorf("variation boards: start %s final %s", v.StartBoard().FEN(), v.FinalBoard().FEN())
	}
}

func TestUnderpromotionRecorded(t *testing.T) {
	g := parseOne(t, "[FEN \"8/P6k/8/8/8/8/8/K7 w - - 0 1\"]\n\n1. a8=N *\n")
	r, err := Game(g, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Moves[0].Move.Promotion != chess.Knight {
		t.Errorf("promotion = %v, want Knight", r.Moves[0].Move.Promotion)
	}
}

func TestReplayPlacementKeyPerPly(t *testing.T) {
	g := parseOne(t, "1. f3 e5 2. g4 Qh4# 0-1\n")
	r, err := Game(g, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range r.Positions {
		b, err := chess.FromFEN(p.FEN)
		if err != nil {
			t.Fatalf("ply %d: %v", i, err)
		}
		if p.Placement != b.PlacementKey() {
			t.Errorf("ply %d: Placement = %#x, PlacementKey(FEN) = %#x", i, p.Placement, b.PlacementKey())
		}
		if p.Placement == 0 {
			t.Errorf("ply %d: Placement is zero", i)
		}
	}
	if r.Positions[0].Placement == r.Positions[1].Placement {
		t.Error("Placement unchanged after 1.f3")
	}
}

// TestEnPassantRecorded pins that replay marks an en passant capture on
// the MoveInfo, so that renderers need not infer it from the previous
// position's FEN, and that an ordinary pawn-takes-pawn is not marked.
func TestEnPassantRecorded(t *testing.T) {
	g := parseOne(t, "1. e4 a6 2. e5 f5 3. exf6 *\n")
	r, err := Game(g, Options{})
	if err != nil {
		t.Fatal(err)
	}
	ep := r.Moves[4]
	if !ep.EnPassant || ep.Captured != chess.Pawn || ep.Move.To.String() != "f6" {
		t.Errorf("exf6: %+v, want EnPassant with a captured pawn", ep)
	}
	for i := 0; i < 4; i++ {
		if r.Moves[i].EnPassant {
			t.Errorf("move %d marked en passant", i)
		}
	}

	g = parseOne(t, "1. e4 d5 2. exd5 *\n")
	r, err = Game(g, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Moves[2].EnPassant || r.Moves[2].Captured != chess.Pawn {
		t.Errorf("exd5: %+v, want a plain pawn capture", r.Moves[2])
	}
}

// TestPlyNumber pins Position.PlyNumber to pgn-extract's play_moves
// convention (apply.c: plies = move_number*2 - (to_move == WHITE ? 1 : 0)):
// the absolute, one-based number of the move about to be played, so the
// standard start is ply 1 and a FEN start derives from the FEN's move
// number and side to move. A fullmove field below 1 is clamped to 1 as
// new_fen_board does.
func TestPlyNumber(t *testing.T) {
	cases := []struct {
		src  string
		want []int // per position, in order
	}{
		{"1. e4 e5 2. Nf3 *\n", []int{1, 2, 3, 4}},
		{"[FEN \"8/8/8/8/8/8/k7/4K2R w K - 0 40\"]\n\n40. O-O *\n", []int{79, 80}},
		{"[FEN \"8/8/8/8/8/8/k7/4K2R b K - 0 40\"]\n\n40... Kb2 41. Ke2 *\n", []int{80, 81, 82}},
		{"[FEN \"7k/3P4/7K/8/8/8/8/8 w - - 6 0\"]\n\nd8=Q *\n", []int{1, 2}},
		{"[FEN \"7k/3P4/7K/8/8/8/8/8 b - - 6 0\"]\n\nKg8 *\n", []int{2, 3}},
	}
	for _, c := range cases {
		g := parseOne(t, c.src)
		r, err := Game(g, Options{})
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		if len(r.Positions) != len(c.want) {
			t.Fatalf("%q: %d positions, want %d", c.src, len(r.Positions), len(c.want))
		}
		for i, want := range c.want {
			if got := r.Positions[i].PlyNumber(); got != want {
				t.Errorf("%q: Positions[%d].PlyNumber() = %d, want %d", c.src, i, got, want)
			}
		}
		// PlyCount is the number of plies completed: one less than the
		// ply number of the final position (apply.c half_moves_played).
		if got, want := r.PlyCount(), c.want[len(c.want)-1]-1; got != want {
			t.Errorf("%q: PlyCount() = %d, want %d", c.src, got, want)
		}
	}
}

// TestFullmoveClampedInFEN pins that a FEN tag whose fullmove field is 0
// starts at move 1 in every typed field and in the FEN replay emits, as
// the C tool's new_fen_board clamps it (oracle: -Wfen prints "... 6 1").
func TestFullmoveClampedInFEN(t *testing.T) {
	g := parseOne(t, "[FEN \"7k/3P4/7K/8/8/8/8/8 w - - 6 0\"]\n\nd8=Q *\n")
	r, err := Game(g, Options{})
	if err != nil {
		t.Fatal(err)
	}
	p := r.Positions[0]
	if p.FullmoveNumber != 1 || p.FEN != "7k/3P4/7K/8/8/8/8/8 w - - 6 1" {
		t.Errorf("start position: %+v", p)
	}
}

// A null move ("--", or the "Z0" spelling pgn-extract's lexer also takes)
// in the main line is rejected unless AllowNullMoves, and is always
// accepted inside a variation (grammar.c parse_move_unit, apply.c
// play_moves: "Null moves (--) only allowed in variations").
func TestReplayNullMoveMainLineNeedsOption(t *testing.T) {
	g := parseOne(t, "1. e4 -- 2. d4 *\n")
	_, err := Game(g, Options{})
	var ill *IllegalMoveError
	if !errors.As(err, &ill) || ill.Ply != 2 || ill.SAN != "--" {
		t.Fatalf("want IllegalMoveError at ply 2 for --, got %v", err)
	}
}

func TestReplayNullMoveMainLineAllowed(t *testing.T) {
	for _, spelling := range []string{"--", "Z0"} {
		g := parseOne(t, "1. e4 "+spelling+" 2. d4 *\n")
		r, err := Game(g, Options{AllowNullMoves: true})
		if err != nil {
			t.Fatalf("%s: %v", spelling, err)
		}
		null := r.Moves[1]
		if !null.Move.IsNull() || null.SAN != "--" || null.IsCheck || null.Captured != chess.NoPiece {
			t.Errorf("%s: null MoveInfo = %+v", spelling, null)
		}
		// apply.c leaves the board as it was: e3 stays, the clock does
		// not advance, only the side to move and move number change.
		if got := r.Positions[2].FEN; got != "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR w KQkq e3 0 2" {
			t.Errorf("%s: FEN after null = %s", spelling, got)
		}
		if got := r.Positions[3].FEN; got != "rnbqkbnr/pppppppp/8/8/3PP3/8/PPP2PPP/RNBQKBNR b KQkq d3 0 2" {
			t.Errorf("%s: FEN after 2. d4 = %s", spelling, got)
		}
		if r.PlyCount() != 3 {
			t.Errorf("%s: PlyCount = %d, want 3", spelling, r.PlyCount())
		}
	}
}

func TestReplayNullMoveInVariationAlwaysAllowed(t *testing.T) {
	g := parseOne(t, "1. e4 (1. d4 --) e5 *\n")
	r, err := Game(g, Options{})
	if err != nil {
		t.Fatal(err)
	}
	v := r.Moves[0].Variations[0]
	if v == nil || len(v.Moves) != 2 || !v.Moves[1].Move.IsNull() {
		t.Fatalf("variation replay = %+v", v)
	}
	if got := v.Positions[2].FEN; got != "rnbqkbnr/pppppppp/8/8/3P4/8/PPP1PPPP/RNBQKBNR w KQkq d3 0 2" {
		t.Errorf("FEN after variation null = %s", got)
	}
}
