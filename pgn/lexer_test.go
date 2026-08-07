// pgn/lexer_test.go
package pgn

import (
	"strings"
	"testing"
)

func lexAll(t *testing.T, src string, opts ParserOptions) []token {
	t.Helper()
	l := newLexer(strings.NewReader(src), opts)
	var toks []token
	for {
		tok, err := l.next()
		if err != nil {
			t.Fatalf("lex error: %v", err)
		}
		if tok.kind == tokEOF {
			return toks
		}
		toks = append(toks, tok)
	}
}

func kinds(toks []token) []tokenKind {
	out := make([]tokenKind, len(toks))
	for i, tk := range toks {
		out[i] = tk.kind
	}
	return out
}

func TestLexFullGame(t *testing.T) {
	src := `[Event "Test"]
[Result "1-0"]

1. e4 e5 2. Nf3! {good} ($2 2. f4 exf4) 2... Nc6 1-0
`
	toks := lexAll(t, src, ParserOptions{})
	want := []tokenKind{
		tokTagName, tokString, tokTagName, tokString,
		tokMoveNumber, tokMove, tokMove, tokMoveNumber, tokMove, tokComment,
		tokRAVStart, tokNAG, tokMoveNumber, tokMove, tokMove, tokRAVEnd,
		tokMoveNumber, tokMove, tokResult,
	}
	got := kinds(toks)
	if len(got) != len(want) {
		t.Fatalf("got %d tokens %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token %d = %v (%q), want %v", i, got[i], toks[i].text, want[i])
		}
	}
	if toks[5].text != "e4" || toks[8].text != "Nf3!" || toks[9].text != "good" {
		t.Errorf("token text wrong: %q %q %q", toks[5].text, toks[8].text, toks[9].text)
	}
}

func TestLexStringEscapes(t *testing.T) {
	toks := lexAll(t, `[Event "a \"quoted\" name"]`, ParserOptions{})
	if toks[1].text != `a "quoted" name` {
		t.Errorf("string = %q", toks[1].text)
	}
}

func TestLexSkipsEscapeAndSemicolonLines(t *testing.T) {
	src := "% import junk\n; whole line comment\n1. e4 *\n"
	toks := lexAll(t, src, ParserOptions{})
	if len(toks) != 3 || toks[0].kind != tokMoveNumber || toks[1].kind != tokMove || toks[2].kind != tokResult {
		t.Fatalf("got %v", kinds(toks))
	}
}

func TestLexResults(t *testing.T) {
	for _, s := range []string{"1-0", "0-1", "1/2-1/2", "*"} {
		toks := lexAll(t, s, ParserOptions{})
		if len(toks) != 1 || toks[0].kind != tokResult || toks[0].text != s {
			t.Errorf("%q lexed as %v", s, toks)
		}
	}
}

func TestLexNestedComments(t *testing.T) {
	src := "{outer {inner} back} *"
	flat := lexAll(t, src, ParserOptions{})
	if flat[0].text != "outer {inner" { // non-nesting: first } closes
		t.Errorf("flat comment = %q", flat[0].text)
	}
	nested := lexAll(t, src, ParserOptions{NestedComments: true})
	if nested[0].text != "outer {inner} back" {
		t.Errorf("nested comment = %q", nested[0].text)
	}
}
