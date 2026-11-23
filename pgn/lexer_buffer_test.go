package pgn

import (
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

// The lexer scans a byte buffer it refills in blocks. These tests feed it
// readers that return a byte or a fragment at a time, so every token kind
// has to survive a refill in the middle of it, and a token longer than the
// initial block has to make the buffer grow. The expected token stream is
// the one a whole-string read produces: the reader's chunking must never
// show in the tokens.

const bufferTestGame = `% escaped line
[Event "Fran\xe7ois \"quoted\" value"]
[Site "A
multi-line value"]

{Start comment
across lines} 1. e4 e5 2. Nf3! {good
one} ($2 2. f4 exf4) 2... Nc6 3. 0-0-0+ O-O $14
% not at line start
1/2-1/2
`

// lexWith lexes src through wrap(reader), returning every token before EOF.
func lexWith(t *testing.T, src string, wrap func(io.Reader) io.Reader) []token {
	t.Helper()
	var r io.Reader = strings.NewReader(src)
	if wrap != nil {
		r = wrap(r)
	}
	l := newLexer(r, ParserOptions{})
	var toks []token
	for {
		tok, err := l.next()
		if err != nil {
			t.Fatalf("lex error: %v", err)
		}
		if tok.kind == tokEOF {
			toks = append(toks, tok)
			return toks
		}
		toks = append(toks, tok)
	}
}

func sameTokens(t *testing.T, name string, got, want []token) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d tokens, want %d\n got %v\nwant %v", name, len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: token %d = %+v, want %+v", name, i, got[i], want[i])
		}
	}
}

func TestLexerTokensSurviveRefills(t *testing.T) {
	want := lexWith(t, bufferTestGame, nil)
	// Sanity-check the whole-string stream itself against the source, so
	// the chunked streams are compared with something known to be right.
	wantKinds := []tokenKind{
		tokTagName, tokString, tokTagName, tokString,
		tokComment, tokMoveNumber, tokMove, tokMove, tokMoveNumber, tokMove, tokComment,
		tokRAVStart, tokNAG, tokMoveNumber, tokMove, tokMove, tokRAVEnd,
		tokMoveNumber, tokMove, tokMoveNumber, tokMove, tokMove, tokNAG,
		tokResult, tokEOF,
	}
	if k := kinds(want); strings.Join(kindNames(k), ",") != strings.Join(kindNames(wantKinds), ",") {
		t.Fatalf("whole-string kinds = %v", k)
	}
	if want[1].text != `Fran\xe7ois \"quoted\" value` || want[1].line != 2 {
		t.Fatalf("string token = %+v", want[1])
	}
	if want[3].text != "A\nmulti-line value" || want[3].line != 3 {
		t.Fatalf("multi-line string token = %+v", want[3])
	}
	if want[4].text != "Start comment across lines" || want[4].line != 6 {
		t.Fatalf("comment token = %+v", want[4])
	}
	if want[10].text != "good one" || want[10].line != 7 {
		t.Fatalf("second comment token = %+v", want[10])
	}
	if want[20].text != "0-0-0+" || want[21].text != "O-O" || want[22].text != "14" {
		t.Fatalf("castling/NAG tokens = %+v %+v %+v", want[20], want[21], want[22])
	}
	if want[23].text != "1/2-1/2" || want[23].line != 10 || want[24].line != 11 {
		t.Fatalf("result/EOF tokens = %+v %+v", want[23], want[24])
	}

	sameTokens(t, "one byte per read", lexWith(t, bufferTestGame, iotest.OneByteReader), want)
	sameTokens(t, "half reads", lexWith(t, bufferTestGame, iotest.HalfReader), want)
	sameTokens(t, "data with EOF", lexWith(t, bufferTestGame, iotest.DataErrReader), want)
}

func kindNames(ks []tokenKind) []string {
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = k.String()
	}
	return out
}

// A token longer than the lexer's initial block (a huge comment, a long
// tag value) must come through whole, with the line count still right
// afterwards.
func TestLexerTokenLongerThanBlock(t *testing.T) {
	long := strings.Repeat("0123456789abcdef\n", 20000) // 340 KB, 20000 newlines
	src := "[Event \"" + long + "\"]\n{" + long + "} 1. e4 *\n"
	for name, wrap := range map[string]func(io.Reader) io.Reader{
		"whole": nil, "half reads": iotest.HalfReader,
	} {
		toks := lexWith(t, src, wrap)
		if len(toks) != 7 {
			t.Fatalf("%s: %d tokens: %v", name, len(toks), kinds(toks))
		}
		if toks[1].kind != tokString || toks[1].text != long {
			t.Errorf("%s: long tag value not preserved (len %d)", name, len(toks[1].text))
		}
		if toks[2].kind != tokComment || toks[2].text != strings.ReplaceAll(long, "\n", " ") {
			t.Errorf("%s: long comment not preserved (len %d)", name, len(toks[2].text))
		}
		if toks[2].line != 20002 || toks[3].line != 40002 || toks[6].line != 40003 {
			t.Errorf("%s: lines = %d %d %d, want 20002 40002 40003", name, toks[2].line, toks[3].line, toks[6].line)
		}
	}
}

// '%' discards the rest of its line only when it is the first byte on the
// line, whatever the reader's chunking: the lexer's line-start state must
// not reset at a refill.
func TestLexerPercentLineStartAcrossRefills(t *testing.T) {
	src := "1. e4 % mid-line stays\n% start of line goes\ne5 *\n"
	want := lexWith(t, src, nil)
	// Mid-line the '%' is skipped as a stray character and the words after
	// it lex as stray tokens; at line start the whole line goes.
	if len(want) != 7 || want[1].text != "e4" || want[2].kind != tokStray || want[3].kind != tokStray ||
		want[4].kind != tokMove || want[4].text != "e5" {
		t.Fatalf("whole-string tokens = %v", want)
	}
	sameTokens(t, "one byte per read", lexWith(t, src, iotest.OneByteReader), want)
}

// Games parsed from the same Scanner never share Move storage: editing one
// game's moves leaves the next game's untouched.
func TestScannerGamesOwnTheirMoves(t *testing.T) {
	s := NewScanner(strings.NewReader(twoGames), ParserOptions{})
	if !s.Next() {
		t.Fatal(s.Err())
	}
	first := s.Game()
	if !s.Next() {
		t.Fatal(s.Err())
	}
	second := s.Game()
	first.Moves[0].SAN = "changed"
	first.Moves[0].AddComment("edited")
	if second.Moves[0].SAN != "d4" || len(second.Moves[0].Annotations) != 0 {
		t.Errorf("second game's move changed with the first's: %+v", second.Moves[0])
	}
	if first.Moves[1].SAN != "e5" || first.Moves[0].Variations() != nil {
		t.Errorf("sibling move changed: %+v", first.Moves[1])
	}
}

// TestScannerYieldsGameWithoutReadingAhead is the pgn-level form of ADR
// 0003's streaming guard: a game whose text is complete is yielded without
// another Read. The result tokens are the sensitive spot, because the
// lexer looks past them for a longer spelling ("1/2" may continue as
// "1/2-1/2", "0-0" as "0-0-0"), and that look must fail on the bytes it
// has rather than wait for more. Each game is written to an io.Pipe up to
// its final newline and the writer then blocks until the game has been
// yielded; a lexer that read ahead would deadlock, and the test time out.
func TestScannerYieldsGameWithoutReadingAhead(t *testing.T) {
	games := []string{
		"[Event \"one\"]\n\n1. e4 e5 1/2\n",
		"[Event \"two\"]\n\n1. e4 e5 1-0\n",
		"[Event \"three\"]\n\n1. e4 e5 0-1\n",
		"[Event \"four\"]\n\n1. e4 e5 1/2-1/2\n",
		"[Event \"five\"]\n\n1. e4 e5 *\n",
	}
	pr, pw := io.Pipe()
	yielded := make(chan struct{})
	go func() {
		defer pw.Close()
		for _, g := range games {
			io.WriteString(pw, g)
			<-yielded
		}
	}()

	done := make(chan []string)
	go func() {
		var events []string
		s := NewScanner(pr, ParserOptions{})
		for s.Next() {
			event, _ := s.Game().Tag("Event")
			events = append(events, event)
			yielded <- struct{}{}
		}
		if err := s.Err(); err != nil {
			t.Error(err)
		}
		done <- events
	}()

	select {
	case events := <-done:
		if got := strings.Join(events, ","); got != "one,two,three,four,five" {
			t.Errorf("events = %s, want one,two,three,four,five", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("scanner blocked reading ahead of a complete game")
	}
}
