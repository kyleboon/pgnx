package output

import "testing"

// Every expected string here is the C oracle's output for the same input
// and flags: printf '...' | pgn-extract <flags> --quiet
func TestAnnotationsRenderInSourceOrder(t *testing.T) {
	const interleaved = "[Result \"*\"]\n\n1. e4 {a} $1 {b} $2 (1. d4) {c} (1. c4) {d} e5 {e} *\n"
	// (A comment after the variation's result would follow a NAG on c5 in
	// the C tool only when c5 has one; with none, as here, both tools
	// agree. See CONFORMANCE.md "Ordered annotations".)
	const inVariation = "[Result \"*\"]\n\n1. e4 $1 $2 {a} {b} $3 e5 (1... c5 {x} 1-0 {y}) {z} 2. Nf3 *\n"
	cases := []struct {
		name string
		src  string
		opts Options
		want string
	}{
		{"nag comment variation interleaving", interleaved, Options{Format: SAN},
			"1. e4 { a } $1 { b } $2 (1. d4) { c } (1. c4) { d } 1... e5 { e } *\n\n"},
		{"no nags keeps the comments that followed them", interleaved, Options{Format: SAN, NoNAGs: true},
			"1. e4 { a } { b } (1. d4) { c } (1. c4) { d } 1... e5 { e } *\n\n"},
		{"no comments", interleaved, Options{Format: SAN, NoComments: true},
			"1. e4 $1 $2 (1. d4) (1. c4) 1... e5 *\n\n"},
		{"no variations keeps the comments that trailed them", interleaved, Options{Format: SAN, NoVariations: true},
			"1. e4 { a } $1 { b } $2 { c } { d } 1... e5 { e } *\n\n"},
		{"generated comments go before the first variation", interleaved, Options{Format: SAN, FENComments: true},
			"1. e4 { a } $1 { b } $2 { rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b\n" +
				"KQkq e3 0 1 } (1. d4 { rnbqkbnr/pppppppp/8/8/3P4/8/PPP1PPPP/RNBQKBNR b KQkq\n" +
				"d3 0 1 }) { c } (1. c4 { rnbqkbnr/pppppppp/8/8/2P5/8/PP1PPPPP/RNBQKBNR b\n" +
				"KQkq c3 0 1 }) { d } 1... e5 { e } {\n" +
				"rnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR w KQkq e6 0 2 } *\n\n"},
		// testdata/golden/infiles/test-splitvariants.pgn without the flag:
		// a comment after a variation's closing paren follows the
		// variation on the move before it, at every depth.
		{"comments after closing parens", "[Result \"*\"]\n\n1. e4 { comment1 } e5 { comment2 }\n( { comment3 } 1... e6 (1... d6) 2. d4 (2. Nf3){ comment4 } ) { comment5 } 2. Nf3\n*\n",
			Options{Format: SAN},
			"1. e4 { comment1 } 1... e5 { comment2 } ( { comment3 } 1... e6 (1... d6) 2.\n" +
				"d4 (2. Nf3) { comment4 }) { comment5 } 2. Nf3 *\n\n"},
		{"result inside a variation", inVariation, Options{Format: SAN},
			"1. e4 $1 $2 { a } { b } $3 1... e5 (1... c5 { x } { y } 1-0) { z } 2. Nf3 *\n\n"},
		{"result inside a variation obeys no results", inVariation, Options{Format: SAN, NoResults: true},
			"1. e4 $1 $2 { a } { b } $3 1... e5 (1... c5 { x } { y }) { z } 2. Nf3\n\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := movetext(t, render(t, c.src, c.opts)); got != c.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}
