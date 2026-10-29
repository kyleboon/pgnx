package filter

import (
	"strings"
	"testing"

	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// replayed parses src as one PGN game and replays it.
func replayed(t *testing.T, src string) (*pgn.Game, *replay.Result) {
	t.Helper()
	s := pgn.NewScanner(strings.NewReader(src), pgn.ParserOptions{})
	if !s.Next() {
		t.Fatalf("no game parsed: %v", s.Err())
	}
	g := s.Game()
	r, err := replay.Game(g, replay.Options{})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	return g, r
}

const foolsMate = "1. f3 e5 2. g4 Qh4# 0-1\n"

func TestMatchCheckmate(t *testing.T) {
	_, r := replayed(t, foolsMate)
	if !matchCheckmate(r) {
		t.Error("fool's mate is checkmate")
	}
	_, r = replayed(t, "1. e4 e5 *\n")
	if matchCheckmate(r) {
		t.Error("not mate")
	}
}

func TestMatchStalemate(t *testing.T) {
	_, r := replayed(t, "[FEN \"7k/8/6Q1/8/8/8/8/K7 w - - 0 1\"]\n\n1. Qf7 1/2-1/2\n")
	if !matchStalemate(r) {
		t.Error("Qf7 stalemates")
	}
	_, r = replayed(t, foolsMate)
	if matchStalemate(r) {
		t.Error("mate is not stalemate")
	}
	_, r = replayed(t, "[FEN \"7k/5Q2/6K1/8/8/8/8/8 b - - 0 1\"]\n\n1/2-1/2\n")
	if !matchStalemate(r) {
		t.Error("zero-move stalemate position")
	}
}

func TestMatchInsufficient(t *testing.T) {
	_, r := replayed(t, "[FEN \"7k/8/8/8/8/8/8/K6R w - - 0 1\"]\n\n1. Rh7+ Kg8 *\n")
	if matchInsufficient(r) {
		t.Error("rook present")
	}
	_, r = replayed(t, "[FEN \"7k/8/8/8/8/8/1r6/K7 w - - 0 1\"]\n\n1. Kxb2 *\n")
	if !matchInsufficient(r) {
		t.Error("K v K")
	}
}

func TestMatchNMoveRuleMarks(t *testing.T) {
	// Clock already at 98; two quiet moves reach 99 then 100.
	src := "[FEN \"4k3/8/8/8/8/8/8/4K2R w K - 98 60\"]\n\n60. Kd1 Kd8 61. Ke1 Ke8 *\n"
	g, r := replayed(t, src)
	if !matchNMoveRule(g, r, 50, "FIFTY") {
		t.Fatal("fifty-move rule should apply")
	}
	want := []int{0, 0, 1, 1} // clocks 99, 100, 101, 102 → plies 2,3,4 qualify
	want = []int{0, 1, 1, 1}
	for i, m := range g.Moves {
		if len(m.Comments) != want[i] {
			t.Errorf("ply %d comments = %v, want %d", i+1, m.Comments, want[i])
		}
	}
	g, r = replayed(t, src)
	if matchNMoveRule(g, r, 75, "X") {
		t.Error("seventy-five should not apply")
	}
	for _, m := range g.Moves {
		if len(m.Comments) != 0 {
			t.Error("no marks when unmatched")
		}
	}
	g, r = replayed(t, src)
	matchNMoveRule(g, r, 50, "")
	if len(g.Moves[1].Comments) != 0 {
		t.Error("empty mark adds nothing")
	}
}

func TestMatchRepetitionMarks(t *testing.T) {
	// Start position recurs after each Ng1 Ng8 pair: count 2 after ply 4,
	// 3 after ply 8, 4 after ply 12. Once the start has occurred three
	// times, every other position in the cycle also reaches three on its
	// next occurrence, so plies 8 through 12 are all marked (as in C).
	src := "1. Nf3 Nf6 2. Ng1 Ng8 3. Nf3 Nf6 4. Ng1 Ng8 5. Nf3 Nf6 6. Ng1 Ng8 *\n"
	g, r := replayed(t, src)
	if !matchRepetition(g, r, 3, "REPEAT") {
		t.Fatal("threefold")
	}
	for i, m := range g.Moves {
		want := 0
		if i+1 >= 8 {
			want = 1
		}
		if len(m.Comments) != want {
			t.Errorf("ply %d comments = %v, want %d", i+1, m.Comments, want)
		}
	}
	g, r = replayed(t, src)
	if matchRepetition(g, r, 5, "X") {
		t.Error("not fivefold")
	}
	g, r = replayed(t, "1. Nf3 Nf6 2. Ng1 Ng8 3. Nf3 Nf6 4. Ng1 Ng8 5. Nf3 Nf6 6. Ng1 Ng8 7. Nf3 Nf6 8. Ng1 Ng8 *\n")
	if !matchRepetition(g, r, 5, "X") || len(g.Moves[15].Comments) != 1 || len(g.Moves[11].Comments) != 0 {
		t.Error("fivefold marks only ply 16")
	}
	g, r = replayed(t, foolsMate)
	if matchRepetition(g, r, 3, "X") {
		t.Error("no repetition")
	}
}

func TestMatchRepetitionFENMark(t *testing.T) {
	src := "1. Nf3 Nf6 2. Ng1 Ng8 3. Nf3 Nf6 4. Ng1 Ng8 *\n"
	g, r := replayed(t, src)
	matchRepetition(g, r, 3, "FEN")
	if got := g.Moves[7].Comments; len(got) != 1 || got[0] != r.Positions[8].FEN {
		t.Errorf("FEN mark = %v", got)
	}
}

func TestMatchUnderpromotion(t *testing.T) {
	_, r := replayed(t, "[FEN \"8/P6k/8/8/8/8/8/K7 w - - 0 1\"]\n\n1. a8=N *\n")
	if !matchUnderpromotion(r) {
		t.Error("knight promotion")
	}
	_, r = replayed(t, "[FEN \"8/P6k/8/8/8/8/8/K7 w - - 0 1\"]\n\n1. a8=Q *\n")
	if matchUnderpromotion(r) {
		t.Error("queen promotion is not under")
	}
}

func TestMatchCommented(t *testing.T) {
	g, _ := replayed(t, "1. e4 e5 (1... c5 2. Nf3 {nested}) 2. Nf3 *\n")
	if !matchCommented(g) {
		t.Error("nested variation comment")
	}
	g, _ = replayed(t, "{prefix} 1. e4 *\n")
	if !matchCommented(g) {
		t.Error("prefix comment")
	}
	g, _ = replayed(t, "1. e4 e5 (1... c5) *\n")
	if matchCommented(g) {
		t.Error("no comments")
	}
}
