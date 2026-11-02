package output

import (
	"bytes"
	"strings"
	"testing"
)

// TestBlackToMoveFEN pins movetext numbering for a game whose FEN has black
// to move, against the C oracle (output.c format_game seeds white_to_move
// from the FEN):
//
//	pgn-extract --quiet -Wsan|-Wlalg|-Welalg
func TestBlackToMoveFEN(t *testing.T) {
	src := "[Event \"T\"]\n[Site \"?\"]\n[Date \"2020.01.01\"]\n[Round \"1\"]\n[White \"A\"]\n[Black \"B\"]\n[Result \"*\"]\n[SetUp \"1\"]\n[FEN \"r3k2r/8/8/8/8/8/8/R3K2R b KQkq - 0 18\"]\n\n18... O-O-O 19. O-O Kd7 {c} 20. Kg2 *\n"
	for _, tc := range []struct {
		f    Format
		want string
	}{
		{SAN, "18... O-O-O 19. O-O Kd7 { c } 20. Kg2 *\n"},
		{LAlg, "18... e8c8 19. e1g1 c8d7 { c } 20. g1g2 *\n"},
		{ELAlg, "18... e8c8 19. e1g1 Kc8d7 { c } 20. Kg1g2 *\n"},
	} {
		g, r := gameAndReplay(t, src)
		var buf bytes.Buffer
		if err := NewWriter(&buf, Options{Format: tc.f}).WriteGame(g, r); err != nil {
			t.Fatal(err)
		}
		if got := buf.String(); !strings.Contains(got, "\n\n"+tc.want) {
			t.Errorf("format %v: got\n%s\nwant movetext %q", tc.f, got, tc.want)
		}
	}
}

// TestFullmoveZeroFEN pins numbering and FEN output for a FEN tag whose
// fullmove field is 0, which the C oracle clamps to 1 (apply.c
// new_fen_board):
//
//	printf '[FEN "7k/3P4/7K/8/8/8/8/8 w - - 6 0"]\n[SetUp "1"]\n[Result "*"]\n\nd8=Q *\n' | pgn-extract --quiet
//	  -> 1. d8=Q# *
//	... | pgn-extract --quiet -Wfen
//	  -> 7k/3P4/7K/8/8/8/8/8 w - - 6 1
//	     3Q3k/8/7K/8/8/8/8/8 b - - 0 1
//	printf '[FEN "7k/3P4/7K/8/8/8/8/8 b - - 6 0"]\n[SetUp "1"]\n[Result "*"]\n\nKg8 *\n' | pgn-extract --quiet
//	  -> 1... Kg8 *
func TestFullmoveZeroFEN(t *testing.T) {
	for _, tc := range []struct {
		src  string
		f    Format
		want string
	}{
		{"[FEN \"7k/3P4/7K/8/8/8/8/8 w - - 6 0\"]\n[SetUp \"1\"]\n[Result \"*\"]\n\nd8=Q *\n", SAN, "1. d8=Q# *\n"},
		{"[FEN \"7k/3P4/7K/8/8/8/8/8 w - - 6 0\"]\n[SetUp \"1\"]\n[Result \"*\"]\n\nd8=Q *\n", FEN, "7k/3P4/7K/8/8/8/8/8 w - - 6 1\n3Q3k/8/7K/8/8/8/8/8 b - - 0 1\n"},
		{"[FEN \"7k/3P4/7K/8/8/8/8/8 b - - 6 0\"]\n[SetUp \"1\"]\n[Result \"*\"]\n\nKg8 *\n", SAN, "1... Kg8 *\n"},
	} {
		g, r := gameAndReplay(t, tc.src)
		var buf bytes.Buffer
		if err := NewWriter(&buf, Options{Format: tc.f}).WriteGame(g, r); err != nil {
			t.Fatal(err)
		}
		if got := buf.String(); !strings.Contains(got, "\n\n"+tc.want) {
			t.Errorf("format %v: got\n%s\nwant movetext %q", tc.f, got, tc.want)
		}
	}
}
