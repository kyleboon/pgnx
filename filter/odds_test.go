package filter

import "testing"

// Odds (--odds) is grammar.c check_for_odds: the game must start from a
// FEN whose middle four ranks are empty, with White to move at move 1,
// and one side must have a different number of pawns, knights, bishops,
// rooks or queens from the other.
func TestMatchOdds(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"knight odds", "[SetUp \"1\"]\n[FEN \"r1bqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1\"]\n\n1. e4 *\n", true},
		{"pawn and move odds", "[SetUp \"1\"]\n[FEN \"rnbqkbnr/ppppp1pp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1\"]\n\n1. e4 *\n", true},
		{"no FEN tag", "1. e4 *\n", false},
		{"standard start as FEN", "[SetUp \"1\"]\n[FEN \"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1\"]\n\n1. e4 *\n", false},
		{"odds but a move already played", "[SetUp \"1\"]\n[FEN \"rnbqkbnr/ppppp1pp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq - 0 1\"]\n\n1... e5 *\n", false},
		{"odds but move number 2", "[SetUp \"1\"]\n[FEN \"rnbqkbnr/ppppp1pp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 2\"]\n\n2. e4 *\n", false},
		{"odds compensated by a different piece", "[SetUp \"1\"]\n[FEN \"rnbqkbnr/ppppp1pp/8/8/8/8/PPPPPPPP/RNBQKBN1 w Qkq - 0 1\"]\n\n1. e4 *\n", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, r := replayed(t, c.src)
			got, err := Evaluate(g, r, &Options{Odds: true})
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("Odds match = %v, want %v", got, c.want)
			}
		})
	}
}
