package dedup

import (
	"strings"
	"testing"

	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// replayed parses the first game of src and replays it.
func replayed(t *testing.T, src string) *replay.Result {
	t.Helper()
	s := pgn.NewScanner(strings.NewReader(src), pgn.ParserOptions{})
	if !s.Next() {
		t.Fatalf("no game parsed: %v", s.Err())
	}
	r, err := replay.Game(s.Game(), replay.Options{})
	if err != nil {
		t.Fatalf("replay %q: %v", src, err)
	}
	return r
}

// games parses and replays every game of src, in order.
func games(t *testing.T, src string) []*replay.Result {
	t.Helper()
	var rs []*replay.Result
	s := pgn.NewScanner(strings.NewReader(src), pgn.ParserOptions{})
	for s.Next() {
		r, err := replay.Game(s.Game(), replay.Options{})
		if err != nil {
			t.Fatalf("replay game %d: %v", len(rs)+1, err)
		}
		rs = append(rs, r)
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	return rs
}

const (
	foolsMate = "1. f3 e5 2. g4 Qh4# 0-1\n"
	// ruyLopez and its transposition reach the same final position by a
	// different route (test-fuzzydepth.pgn games 1 and 4).
	ruyLopez      = "1. e4 e5 2. Nf3 Nc6 3. Bb5 *\n"
	ruyTransposed = "1. e4 e5 2. Bb5 Nc6 3. Nf3 *\n"
	// fenGame starts at move 21 with White to move: its first ply is 41.
	fenGame = "[SetUp \"1\"]\n[FEN \"r1bq1rk1/pp3ppp/2n2n2/3pp3/1b1P4/2NBPN2/PP3PPP/R2Q1RK1 w - - 0 21\"]\n\n21. dxe5 Nxe5 22. Nxe5 *\n"
)
