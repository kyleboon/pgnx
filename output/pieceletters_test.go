package output

import (
	"strings"
	"testing"

	"github.com/kyleboon/pgnx/chess"
)

// TestParsePieceLetters pins the -Wsan<letters> suffix grammar of apply.c
// set_output_piece_characters: six letters in the order pawn, knight,
// bishop, rook, queen, king, each either one character or the two-character
// form "X+Y" (spelled "XY" in the output). The C tool works on bytes, so it
// rejects multi-byte UTF-8 letters as "too many"; pgnx counts runes.
func TestParsePieceLetters(t *testing.T) {
	cases := []struct {
		in   string
		want [chess.King + 1]string // indexed by chess.PieceType
		err  bool
	}{
		{"PNBRQK", [chess.King + 1]string{"", "P", "N", "B", "R", "Q", "K"}, false},
		{"BSLTDK", [chess.King + 1]string{"", "B", "S", "L", "T", "D", "K"}, false},
		{"pnbrqk", [chess.King + 1]string{"", "p", "n", "b", "r", "q", "k"}, false},
		{"P+pN+nB+bR+rQ+qK+k", [chess.King + 1]string{"", "Pp", "Nn", "Bb", "Rr", "Qq", "Kk"}, false},
		{"ПКСЛФК+р", [chess.King + 1]string{"", "П", "К", "С", "Л", "Ф", "Кр"}, false},
		{"", [chess.King + 1]string{}, true},
		{"BSLTD", [chess.King + 1]string{}, true},
		{"BSLTDKX", [chess.King + 1]string{}, true},
		{"P+", [chess.King + 1]string{}, true},
		{"PNBRQK+", [chess.King + 1]string{}, true},
	}
	for _, c := range cases {
		got, err := ParsePieceLetters(c.in)
		if (err != nil) != c.err {
			t.Errorf("ParsePieceLetters(%q): err = %v, want error %v", c.in, err, c.err)
			continue
		}
		if err != nil {
			continue
		}
		for pt := chess.Pawn; pt <= chess.King; pt++ {
			if l := got.Letter(pt); l != c.want[pt] {
				t.Errorf("ParsePieceLetters(%q).Letter(%v) = %q, want %q", c.in, pt, l, c.want[pt])
			}
		}
	}
}

// The zero PieceLetters is the C tool's default English alphabet.
func TestPieceLettersZeroIsEnglish(t *testing.T) {
	var p PieceLetters
	want := map[chess.PieceType]string{chess.Pawn: "P", chess.Knight: "N", chess.Bishop: "B", chess.Rook: "R", chess.Queen: "Q", chess.King: "K"}
	for pt, l := range want {
		if got := p.Letter(pt); got != l {
			t.Errorf("zero PieceLetters.Letter(%v) = %q, want %q", pt, got, l)
		}
	}
}

// TestPieceLettersRendering pins which formats spell pieces with
// Options.PieceLetters, against the C oracle (from ~/code/pgn-extract,
// input "1. e4 d5 2. exd5 c6 3. dxc6 Nf6 4. cxb7 Nbd7 5. bxa8=N Qc7 (5...
// Qa5 6. Nc7+ Kd8) 6. Nxc7+ Kd8 *"):
//
//	-WsanBSLTDK   -> 1. e4 d5 2. exd5 c6 3. dxc6 Sf6 4. cxb7 Sbd7 5. bxa8=S Dc7 (5... Da5 6. Sc7+ Kd8) 6. Sxc7+ Kd8 *
//	-WelalgBSLTDK -> 1. e2e4 d7d5 2. e4d5 c7c6 3. d5c6 Sg8f6 4. c6b7 Sb8d7 5. b7a8S Dd8c7 (5... Dd8a5 6. Sa8c7+ Ke8d8) 6. Sa8c7+ Ke8d8 *
//	-WxlalgBSLTDK -> 1. e2-e4 d7-d5 2. e4xd5 c7-c6 3. d5xc6 Sg8-f6 4. c6xb7 Sb8-d7 5. b7xa8S Dd8-c7 (5... Dd8-a5 6. Sa8-c7+ Ke8-d8) 6. Sa8xc7+ Ke8-d8 *
//	-WlalgBSLTDK  -> 1. e2e4 d7d5 2. e4d5 c7c6 3. d5c6 g8f6 4. c6b7 b8d7 5. b7a8N d8c7 (5... d8a5 6. a8c7+ e8d8) 6. a8c7+ e8d8 *
//	-WhalgBSLTDK  -> 1. e2-e4 d7-d5 2. e4-d5 c7-c6 3. d5-c6 g8-f6 4. c6-b7 b8-d7 5. b7-a8N d8-c7 (5... d8-a5 6. a8-c7+ e8-d8) 6. a8-c7+ e8-d8 *
//	-WuciBSLTDK   -> e2e4 d7d5 e4d5 c7c6 d5c6 g8f6 c6b7 b8d7 b7a8N d8c7 a8c7 e8d8 *
//	-WsanP+pN+nB+bR+rQ+qK+k -> 1. e4 d5 2. exd5 c6 3. dxc6 Nnf6 4. cxb7 Nnbd7 5. bxa8=Nn Qqc7 (5... Qqa5 6. Nnc7+ Kkd8) 6. Nnxc7+ Kkd8 *
//
// output.c set_output_format calls set_output_piece_characters only for
// san, elalg and xlalg (and xolalg); lalg, halg and uci keep the English
// promotion letter whatever suffix was given.
func TestPieceLettersRendering(t *testing.T) {
	const src = "[Result \"*\"]\n\n1. e4 d5 2. exd5 c6 3. dxc6 Nf6 4. cxb7 Nbd7 5. bxa8=N Qc7 (5... Qa5 6. Nc7+ Kd8) 6. Nxc7+ Kd8 *\n"
	german := mustPieceLetters(t, "BSLTDK")
	cases := []struct {
		f       Format
		letters PieceLetters
		want    string
	}{
		{SAN, german, "1. e4 d5 2. exd5 c6 3. dxc6 Sf6 4. cxb7 Sbd7 5. bxa8=S Dc7 (5... Da5 6. Sc7+ Kd8) 6. Sxc7+ Kd8 *"},
		{ELAlg, german, "1. e2e4 d7d5 2. e4d5 c7c6 3. d5c6 Sg8f6 4. c6b7 Sb8d7 5. b7a8S Dd8c7 (5... Dd8a5 6. Sa8c7+ Ke8d8) 6. Sa8c7+ Ke8d8 *"},
		{XLAlg, german, "1. e2-e4 d7-d5 2. e4xd5 c7-c6 3. d5xc6 Sg8-f6 4. c6xb7 Sb8-d7 5. b7xa8S Dd8-c7 (5... Dd8-a5 6. Sa8-c7+ Ke8-d8) 6. Sa8xc7+ Ke8-d8 *"},
		{LAlg, german, "1. e2e4 d7d5 2. e4d5 c7c6 3. d5c6 g8f6 4. c6b7 b8d7 5. b7a8N d8c7 (5... d8a5 6. a8c7+ e8d8) 6. a8c7+ e8d8 *"},
		{HAlg, german, "1. e2-e4 d7-d5 2. e4-d5 c7-c6 3. d5-c6 g8-f6 4. c6-b7 b8-d7 5. b7-a8N d8-c7 (5... d8-a5 6. a8-c7+ e8-d8) 6. a8-c7+ e8-d8 *"},
		{UCI, german, "e2e4 d7d5 e4d5 c7c6 d5c6 g8f6 c6b7 b8d7 b7a8N d8c7 a8c7 e8d8 *"},
		{SAN, mustPieceLetters(t, "P+pN+nB+bR+rQ+qK+k"), "1. e4 d5 2. exd5 c6 3. dxc6 Nnf6 4. cxb7 Nnbd7 5. bxa8=Nn Qqc7 (5... Qqa5 6. Nnc7+ Kkd8) 6. Nnxc7+ Kkd8 *"},
		// The zero value is the English default.
		{SAN, PieceLetters{}, "1. e4 d5 2. exd5 c6 3. dxc6 Nf6 4. cxb7 Nbd7 5. bxa8=N Qc7 (5... Qa5 6. Nc7+ Kd8) 6. Nxc7+ Kd8 *"},
	}
	for _, c := range cases {
		got := render(t, src, Options{Format: c.f, PieceLetters: c.letters, LineWidth: 500})
		if !strings.Contains(got, c.want+"\n") {
			t.Errorf("format %v, letters %v:\n%s\nwant line %q", c.f, c.letters, got, c.want)
		}
	}
}

// Source output is the SAN as written; letters never touch it
// (set_output_format never sets them for the SOURCE format).
func TestPieceLettersLeaveSourceAlone(t *testing.T) {
	got := render(t, scandi, Options{Format: Source, PieceLetters: mustPieceLetters(t, "BSLTDK")})
	if !strings.Contains(got, "1. e4 d5 2. exd5 Qxd5 *") {
		t.Errorf("Source with piece letters:\n%s", got)
	}
}

func mustPieceLetters(t *testing.T, s string) PieceLetters {
	t.Helper()
	p, err := ParsePieceLetters(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
