package replay

import (
	"errors"
	"strings"
	"testing"

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
