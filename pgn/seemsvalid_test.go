package pgn

import (
	"strings"
	"testing"
)

func TestMoveSeemsValid(t *testing.T) {
	valid := []string{"e4", "exd5", "e8=Q+", "bxa8=N#", "Nf3", "Nbd7", "N1f3", "Qh4xe1", "Rae1", "R1e1",
		"e2-e4", "Ng1-f3", "O-O", "O-O-O+", "0-0", "o-o-o", "exd6ep", "exd6e.p.", "Bxe5+"}
	for _, s := range valid {
		if !moveSeemsValid(s) {
			t.Errorf("moveSeemsValid(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "N", "R", "Q", "-O", "O", "resigns", "e", "Nf", "x4", "Nf3z", "Kx"}
	for _, s := range invalid {
		if moveSeemsValid(s) {
			t.Errorf("moveSeemsValid(%q) = true, want false", s)
		}
	}
}

func TestStrayTokensSkipped(t *testing.T) {
	src := "[Event \"x\"]\n\n1. e4 e5 2. N Nf3 - Nc6 +/- 1/2-1/2\n"
	s := NewScanner(strings.NewReader(src), ParserOptions{})
	if !s.Next() {
		t.Fatal("no game")
	}
	g := s.Game()
	if len(g.Moves) != 4 || g.Result != "1/2-1/2" {
		t.Fatalf("moves=%d result=%q, want 4 moves and 1/2-1/2", len(g.Moves), g.Result)
	}
	if n := len(g.SkippedTokens()); n != 3 {
		t.Errorf("skipped %d tokens, want 3 (N, -, -)", n)
	}
}
