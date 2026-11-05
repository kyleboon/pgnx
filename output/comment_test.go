package output

import (
	"bytes"
	"testing"
)

// TestWriteComment: the lines grammar.c deal_with_game writes into the
// duplicates file ("{ From: ... }", "{ First found in: ... }") through
// print_str and terminate_line: one comment on a line of its own.
func TestWriteComment(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN})
	if err := w.WriteComment("From: games/a.pgn"); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "{ From: games/a.pgn }\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestWriteCommentJSON: under --json the C tool would write the comment
// raw into the array and break it; pgnx writes nothing (CONFORMANCE.md).
func TestWriteCommentJSON(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN, JSON: true})
	if err := w.WriteComment("From: a.pgn"); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("JSON output got %q, want nothing", buf.String())
	}
}
