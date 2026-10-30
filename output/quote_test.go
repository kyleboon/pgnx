package output

import "testing"

// Expected strings are what pgn-extract's output.c emits for the same raw
// source text: bytes verbatim, escape sequences untouched.
func TestQuoteTagValueMatchesOracle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"GM x", "\"GM x\""},
		{"a\tb", "\"a\tb\""},
		{`a\"b\\c\d`, `"a\"b\\c\d"`},
		{`q\\\"r`, `"q\\\"r"`},
		{`bare"quote`, `"bare\"quote"`},
		{"all \t\\\\\\\"", "\"all \t\\\\\\\"\""},
	}
	for _, c := range cases {
		if got := quoteTagValue(c.in); got != c.want {
			t.Errorf("quoteTagValue(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
