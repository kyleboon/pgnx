package pgn

import (
	"strings"
	"testing"
)

// Raw non-UTF-8 bytes (Latin-1) in tag values and comments must pass through
// the lexer unchanged rather than being replaced with U+FFFD, matching the
// byte-oriented pgn-extract lexer.
func TestLexerPreservesRawBytes(t *testing.T) {
	in := "[Event \"Fran\xe7ois \xc3\xa9\"]\n[White \"Andr\xe9\"]\n\n1. e4 {caf\xe9 \xb4} e5 1-0\n"
	s := NewScanner(strings.NewReader(in), ParserOptions{})
	if !s.Next() {
		t.Fatalf("no game: %v", s.Err())
	}
	g := s.Game()
	found := 0
	for _, tag := range g.Tags {
		switch tag.Value {
		case "Fran\xe7ois \xc3\xa9", "Andr\xe9":
			found++
		}
		if strings.Contains(tag.Value, "\xef\xbf\xbd") {
			t.Errorf("tag %s contains U+FFFD: %q", tag.Name, tag.Value)
		}
	}
	if found != 2 {
		t.Errorf("tag values not preserved byte-for-byte: %+v", g.Tags)
	}
	if len(g.Moves) == 0 || len(g.Moves[0].Comments) != 1 || g.Moves[0].Comments[0] != "caf\xe9 \xb4" {
		t.Errorf("comment not preserved: %+v", g.Moves)
	}
}
