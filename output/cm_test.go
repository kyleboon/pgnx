package output

import "testing"

// TestCMOracle pins the ChessMaster layout against pgn-extract -Wcm
// --quiet (output.c output_cm_game: a WHITE:/BLACK: header, a blank line,
// one move pair per line in 3-wide move numbers and 15-wide move columns,
// comments and the result as ";" comments, a trailing newline) and its
// edge cases. Each case's input was fed to the C binary (with -w40 for
// the narrow-width case and -C for the NoComments one) and its stdout
// copied here byte for byte.
func TestCMOracle(t *testing.T) {
	cases := []struct {
		name string
		src  string
		opts Options
		want string
	}{
		{"black resigns after white's move", "[White \"Fischer, R.\"]\n[Black \"Spassky, B.\"]\n[Result \"1-0\"]\n\n1. e4 e5 2. Nf3 Nc6 3. Bb5 1-0\n", Options{},
			"WHITE: Fischer, R.\nBLACK: Spassky, B.\n\n" +
				"  1. e4             e5             \n" +
				"  2. Nf3            Nc6            \n" +
				"  3. Bb5            ...            ; and black resigns\n" +
				"     ...            \n\n"},
		{"white resigns after a full pair", "[Result \"0-1\"]\n\n1. e4 e5 2. Nf3 Nc6 3. Bb5 a6 0-1\n", Options{},
			"WHITE: \nBLACK: \n\n" +
				"  1. e4             e5             \n" +
				"  2. Nf3            Nc6            \n" +
				"  3. Bb5            a6             ; and white resigns\n\n"},
		{"draw", "[Result \"1/2-1/2\"]\n\n1. e4 e5 1/2-1/2\n", Options{},
			"WHITE: \nBLACK: \n\n  1. e4             e5             ; draw\n\n"},
		{"star after white's move", "[Result \"*\"]\n\n1. e4 e5 2. Nf3 *\n", Options{},
			"WHITE: \nBLACK: \n\n" +
				"  1. e4             e5             \n" +
				"  2. Nf3            ...            ; incomplete result\n" +
				"     ...            \n\n"},
		// Checkmate is "++", no result comment, and white's mating move
		// leaves the game without its blank line.
		{"white mates", "[Result \"1-0\"]\n\n1. e4 e5 2. Qh5 Nc6 3. Bc4 Nf6 4. Qxf7# 1-0\n", Options{},
			"WHITE: \nBLACK: \n\n" +
				"  1. e4             e5             \n" +
				"  2. Qh5            Nc6            \n" +
				"  3. Bc4            Nf6            \n" +
				"  4. Qxf7++         \n"},
		{"black mates", "[Result \"0-1\"]\n\n1. f3 e5 2. g4 Qh4# 0-1\n", Options{},
			"WHITE: \nBLACK: \n\n" +
				"  1. f3             e5             \n" +
				"  2. g4             Qh4++          \n\n"},
		{"check kept under NoChecks", "[Result \"*\"]\n\n1. e4 d5 2. exd5 Qxd5 3. Nc3 Qe5+ *\n", Options{NoChecks: true},
			"WHITE: \nBLACK: \n\n" +
				"  1. e4             d5             \n" +
				"  2. exd5           Qxd5           \n" +
				"  3. Nc3            Qe5+           ; incomplete result\n\n"},
		{"comments", "[Result \"1-0\"]\n\n{Prefix comment here} 1. e4 {after white} e5 {after black} 2. Nf3 {two} {comments} Nc6 3. Bb5 {last with result} 1-0\n", Options{},
			"WHITE: \nBLACK: \n\n" +
				"; Prefix comment here\n" +
				"  1. e4             ...            ; after white\n" +
				"     ...            e5             ; after black\n" +
				"  2. Nf3            ...            ; two comments\n" +
				"     ...            Nc6            \n" +
				"  3. Bb5            ...            ; last with result\n" +
				"     ...                           ; and black resigns\n\n"},
		{"comment on black's last move", "[Result \"0-1\"]\n\n1. e4 e5 {last black} 0-1\n", Options{},
			"WHITE: \nBLACK: \n\n" +
				"  1. e4             e5             ; last black\n" +
				"                                   ; and white resigns\n\n"},
		{"comment on white's mate", "[Result \"1-0\"]\n\n1. e4 e5 2. Qh5 Nc6 3. Bc4 Nf6 4. Qxf7# {mate!} 1-0\n", Options{},
			"WHITE: \nBLACK: \n\n" +
				"  1. e4             e5             \n" +
				"  2. Qh5            Nc6            \n" +
				"  3. Bc4            Nf6            \n" +
				"  4. Qxf7++         ...            ; mate!\n" +
				"     ...            \n"},
		{"comment on black's mate", "[Result \"0-1\"]\n\n1. f3 e5 2. g4 Qh4# {mate!} 0-1\n", Options{},
			"WHITE: \nBLACK: \n\n" +
				"  1. f3             e5             \n" +
				"  2. g4             Qh4++          ; mate!\n\n"},
		{"long comment wraps at the comment column", "[Result \"*\"]\n\n1. e4 {This is a very long comment that should certainly wrap around the line because it goes well beyond seventy five characters in total length and then some more words} e5 2. Nf3 *\n", Options{},
			"WHITE: \nBLACK: \n\n" +
				"  1. e4             ...            ; This is a very long comment that\n" +
				"                                   ; should certainly wrap around the line\n" +
				"                                   ; because it goes well beyond seventy\n" +
				"                                   ; five characters in total length and\n" +
				"                                   ; then some more words\n" +
				"     ...            e5             \n" +
				"  2. Nf3            ...            ; incomplete result\n" +
				"     ...            \n\n"},
		{"long prefix comment wraps at column 0", "[Result \"*\"]\n\n{This is a very long prefix comment that should certainly wrap around the line because it goes well beyond seventy five characters in total length and then some more words} 1. e4 e5 *\n", Options{},
			"WHITE: \nBLACK: \n\n" +
				"; This is a very long prefix comment that should certainly wrap around the\n" +
				"; line because it goes well beyond seventy five characters in total length\n" +
				"; and then some more words\n" +
				"  1. e4             e5             ; incomplete result\n\n"},
		{"narrow width wraps before the first word", "[Result \"*\"]\n\n1. e4 {This is a very long comment} e5 *\n", Options{LineWidth: 40},
			"WHITE: \nBLACK: \n\n" +
				"  1. e4             ...            ;\n" +
				"                                   ; This\n" +
				"                                   ; is\n" +
				"                                   ; a\n" +
				"                                   ; very\n" +
				"                                   ; long\n" +
				"                                   ; comment\n" +
				"     ...            e5             ; incomplete result\n\n"},
		// Line breaks and runs of spaces separate words; a tab does not.
		{"multi-line comment", "[Result \"*\"]\n\n1. e4 {line one\r\nline two   with   spaces\ttab} e5 *\n", Options{},
			"WHITE: \nBLACK: \n\n" +
				"  1. e4             ...            ; line one line two with spaces\ttab\n" +
				"     ...            e5             ; incomplete result\n\n"},
		{"two prefix comments share one line", "[Result \"*\"]\n\n{a b} {c d} 1. e4 e5 *\n", Options{},
			"WHITE: \nBLACK: \n\n; a b c d\n  1. e4             e5             ; incomplete result\n\n"},
		{"empty comments", "[Result \"*\"]\n\n{} 1. e4 {} e5 *\n", Options{},
			"WHITE: \nBLACK: \n\n;\n  1. e4             ...            ;\n     ...            e5             ; incomplete result\n\n"},
		// -C drops the move comments but leaves a bare ";" for the prefix.
		{"NoComments", "[Result \"*\"]\n\n{a b} {c d} 1. e4 {after white} e5 {after black} *\n", Options{NoComments: true},
			"WHITE: \nBLACK: \n\n;\n  1. e4             e5             ; incomplete result\n\n"},
		{"zero-move game", "[White \"A\"]\n[Result \"1-0\"]\n\n1-0\n", Options{},
			"WHITE: A\nBLACK: \n\n\n"},
		// NAGs and variations are dropped, and so is a comment after a
		// NAG or a variation: the C tool's comment_list holds only the
		// comments written directly after the move.
		{"variations and NAGs dropped", "[Result \"*\"]\n\n1. e4 $1 (1. d4 d5) e5 $2 {c} (1... c5 {sicilian}) 2. Nf3 *\n", Options{},
			"WHITE: \nBLACK: \n\n" +
				"  1. e4             e5             \n" +
				"  2. Nf3            ...            ; incomplete result\n" +
				"     ...            \n\n"},
		{"only the comments directly after a move", "[Result \"*\"]\n\n1. e4 {a} $1 {b} (1. d4 d5) {c} e5 (1... c5) {d} 2. Nf3 *\n", Options{},
			"WHITE: \nBLACK: \n\n" +
				"  1. e4             ...            ; a\n" +
				"     ...            e5             \n" +
				"  2. Nf3            ...            ; incomplete result\n" +
				"     ...            \n\n"},
		{"tag escapes written as stored", "[White \"A \\\"B\\\" C\\\\D\"]\n[Black \"O'Kelly\"]\n[Result \"*\"]\n\n1. e4 *\n", Options{},
			"WHITE: A \\\"B\\\" C\\\\D\nBLACK: O'Kelly\n\n  1. e4             ...            ; incomplete result\n     ...            \n\n"},
		{"FEN start, white to move", "[FEN \"4k3/8/8/8/8/8/4P3/4K3 w - - 0 40\"]\n[SetUp \"1\"]\n[Result \"*\"]\n\n40. e4 Kd7 41. Kd2 *\n", Options{},
			"WHITE: \nBLACK: \n\n" +
				" 40. e4             Kd7            \n" +
				" 41. Kd2            ...            ; incomplete result\n" +
				"     ...            \n\n"},
		{"FEN start, black to move", "[FEN \"4k3/8/8/8/8/8/4P3/4K3 b - - 0 40\"]\n[SetUp \"1\"]\n[Result \"*\"]\n\n40... Kd7 41. e4 Ke6 *\n", Options{},
			"WHITE: \nBLACK: \n\n" +
				"Kd7            \n" +
				" 41. e4             Ke6            ; incomplete result\n\n"},
		{"rendering options ignored", "[White \"W\"]\n[Result \"1-0\"]\n\n1. e4 {x} e5 2. Nf3 Nc6 3. Bb5 1-0\n",
			Options{NoResults: true, NoMoveNumbers: true, NoTags: true, CommentLines: true, FENComments: true, HashComments: true, Evaluation: true, FinalFEN: true, PlyLimit: 2, HasPlyLimit: true},
			"WHITE: W\nBLACK: \n\n" +
				"  1. e4             ...            ; x\n" +
				"     ...            e5             \n" +
				"  2. Nf3            Nc6            \n" +
				"  3. Bb5            ...            ; and black resigns\n" +
				"     ...            \n\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.opts.Format = CM
			if got := render(t, c.src, c.opts); got != c.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}
