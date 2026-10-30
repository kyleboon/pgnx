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
