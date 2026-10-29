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

func TestFinalBoardRoundTrip(t *testing.T) {
	g := parseOne(t, "1. f3 e5 2. g4 Qh4# 0-1\n")
	r, err := Game(g, Options{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.FinalBoard()
	if err != nil {
		t.Fatal(err)
	}
	if b.FEN() != r.Positions[4].FEN || !b.IsCheckmate() {
		t.Errorf("FinalBoard FEN = %s", b.FEN())
	}
	if _, err := (&Result{}).FinalBoard(); err == nil {
		t.Error("empty result should fail")
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
