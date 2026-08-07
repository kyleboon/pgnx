package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

func gameAndReplay(t *testing.T, src string) (*pgn.Game, *replay.Result) {
	t.Helper()
	s := pgn.NewScanner(strings.NewReader(src), pgn.ParserOptions{})
	if !s.Next() {
		t.Fatalf("parse: %v", s.Err())
	}
	g := s.Game()
	r, err := replay.Game(g, replay.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return g, r
}

const fools = "[Event \"Casual\"]\n[Site \"?\"]\n[Date \"2020.01.01\"]\n[Round \"1\"]\n[White \"A\"]\n[Black \"B\"]\n[Result \"0-1\"]\n\n1. f3 e5 2. g4 {oops} Qh4# 0-1\n"

// TestWriteSAN is pinned byte-for-byte against the C oracle:
//
//	printf '...' | pgn-extract --quiet
func TestWriteSAN(t *testing.T) {
	g, r := gameAndReplay(t, fools)
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN})
	if err := w.WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	want := `[Event "Casual"]
[Site "?"]
[Date "2020.01.01"]
[Round "1"]
[White "A"]
[Black "B"]
[Result "0-1"]

1. f3 e5 2. g4 { oops } 2... Qh4# 0-1

`
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

func TestStripFlags(t *testing.T) {
	g, r := gameAndReplay(t, fools)
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN, NoComments: true, NoMoveNumbers: true, NoResults: true})
	if err := w.WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	// Isolate the movetext (after the blank line following tags) since the
	// tag section legitimately contains a "1." substring inside the Date
	// tag ("2020.01.01").
	s := buf.String()
	_, movetext, ok := strings.Cut(s, "\n\n")
	if !ok {
		t.Fatalf("no blank line separating tags from movetext:\n%s", s)
	}
	if strings.Contains(movetext, "{") || strings.Contains(movetext, "1.") || strings.Contains(movetext, "0-1") {
		t.Errorf("strip flags ignored:\n%s", movetext)
	}
}

// TestLineWrap is pinned byte-for-byte against the C oracle (LineWidth 20):
//
//	printf '[Result "*"]\n\n1. e4 e5 2. Nf3 Nc6 3. Bb5 a6 4. Ba4 Nf6 5. O-O Be7 6. Re1 b5 7. Bb3 d6 8. c3 O-O *\n' \
//	  | pgn-extract --quiet -w20
func TestLineWrap(t *testing.T) {
	long := "[Result \"*\"]\n\n1. e4 e5 2. Nf3 Nc6 3. Bb5 a6 4. Ba4 Nf6 5. O-O Be7 6. Re1 b5 7. Bb3 d6 8. c3 O-O *\n"
	g, r := gameAndReplay(t, long)
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN, LineWidth: 20})
	if err := w.WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	want := `[Event "?"]
[Site "?"]
[Date "????.??.??"]
[Round "?"]
[White "?"]
[Black "?"]
[Result "*"]

1. e4 e5 2. Nf3 Nc6
3. Bb5 a6 4. Ba4 Nf6
5. O-O Be7 6. Re1 b5
7. Bb3 d6 8. c3 O-O
*

`
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
	for _, line := range strings.Split(buf.String(), "\n") {
		if len(line) > 20 && !strings.HasPrefix(line, "[") {
			t.Errorf("line over width: %q", line)
		}
	}
}

func TestRoster(t *testing.T) {
	g, r := gameAndReplay(t, "[Event \"E\"]\n[Annotator \"X\"]\n[Result \"*\"]\n\n1. e4 *\n")
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN, SevenTagRoster: true})
	if err := w.WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "Annotator") {
		t.Error("seven-tag roster leaked extra tag")
	}
	want := "[Event \"E\"]\n[Site \"?\"]\n[Date \"????.??.??\"]\n[Round \"?\"]\n[White \"?\"]\n[Black \"?\"]\n[Result \"*\"]\n\n1. e4 *\n\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

// TestNonRosterTagOrdering pins that, without SevenTagRoster, extra tags are
// emitted after the seven roster tags in their original appearance order,
// regardless of where Result appeared among them in the source. Oracle:
//
//	printf '[Event "E"]\n[Site "S"]\n[Date "2020.01.01"]\n[Round "1"]\n[White "W"]\n[Black "B"]\n[Annotator "X"]\n[Result "*"]\n[ECO "C00"]\n\n1. e4 *\n' \
//	  | pgn-extract --quiet
func TestNonRosterTagOrdering(t *testing.T) {
	src := "[Event \"E\"]\n[Site \"S\"]\n[Date \"2020.01.01\"]\n[Round \"1\"]\n[White \"W\"]\n[Black \"B\"]\n[Annotator \"X\"]\n[Result \"*\"]\n[ECO \"C00\"]\n\n1. e4 *\n"
	g, r := gameAndReplay(t, src)
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN})
	if err := w.WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	want := "[Event \"E\"]\n[Site \"S\"]\n[Date \"2020.01.01\"]\n[Round \"1\"]\n[White \"W\"]\n[Black \"B\"]\n[Result \"*\"]\n[ECO \"C00\"]\n[Annotator \"X\"]\n\n1. e4 *\n\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

// TestMissingRosterTags pins that missing roster tags are filled with the
// appropriate default even when no SevenTagRoster option is set. Oracle:
//
//	printf '[Result "*"]\n\n1. e4 e5 2. Nf3 Nc6 3. Bb5 a6 4. Ba4 Nf6 5. O-O Be7 6. Re1 b5 7. Bb3 d6 8. c3 O-O *\n' \
//	  | pgn-extract --quiet -w20
func TestMissingRosterTags(t *testing.T) {
	g, r := gameAndReplay(t, "[Result \"*\"]\n\n1. e4 *\n")
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN})
	if err := w.WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	want := "[Event \"?\"]\n[Site \"?\"]\n[Date \"????.??.??\"]\n[Round \"?\"]\n[White \"?\"]\n[Black \"?\"]\n[Result \"*\"]\n\n1. e4 *\n\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

// TestNoResultsKeepsTag pins --noresults: the Result tag stays in the header
// but the trailing result token is dropped from the movetext. Oracle:
//
//	printf '[Result "*"]\n\n1. e4 e5 *\n' | pgn-extract --quiet --noresults
func TestNoResultsKeepsTag(t *testing.T) {
	g, r := gameAndReplay(t, "[Result \"*\"]\n\n1. e4 e5 *\n")
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN, NoResults: true})
	if err := w.WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	want := "[Event \"?\"]\n[Site \"?\"]\n[Date \"????.??.??\"]\n[Round \"?\"]\n[White \"?\"]\n[Black \"?\"]\n[Result \"*\"]\n\n1. e4 e5\n\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

// TestBlackRenumberNAGvsComment pins the distinction: a NAG on white's move
// does NOT force a black move renumber, but a comment does. Oracle:
//
//	printf '[Result "*"]\n\n1. e4 e5 2. Nf3 $1 Nc6 3. Bb5 a6 *\n' | pgn-extract --quiet
//	printf '[Result "*"]\n\n1. e4 e5 2. Nf3 {c} Nc6 3. Bb5 a6 *\n' | pgn-extract --quiet
func TestBlackRenumberNAGvsComment(t *testing.T) {
	t.Run("NAG does not renumber", func(t *testing.T) {
		g, r := gameAndReplay(t, "[Result \"*\"]\n\n1. e4 e5 2. Nf3 $1 Nc6 3. Bb5 a6 *\n")
		var buf bytes.Buffer
		w := NewWriter(&buf, Options{Format: SAN})
		if err := w.WriteGame(g, r); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(buf.String(), "2... Nc6") {
			t.Errorf("NAG incorrectly triggered renumber:\n%s", buf.String())
		}
		if !strings.Contains(buf.String(), "2. Nf3 $1 Nc6") {
			t.Errorf("unexpected movetext:\n%s", buf.String())
		}
	})

	t.Run("comment renumbers", func(t *testing.T) {
		g, r := gameAndReplay(t, "[Result \"*\"]\n\n1. e4 e5 2. Nf3 {c} Nc6 3. Bb5 a6 *\n")
		var buf bytes.Buffer
		w := NewWriter(&buf, Options{Format: SAN})
		if err := w.WriteGame(g, r); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "2. Nf3 { c } 2... Nc6") {
			t.Errorf("comment did not trigger renumber:\n%s", buf.String())
		}
	})
}

// TestVariationForcesRenumber pins that a variation attached to white's move
// forces the next black move to be explicitly numbered. Oracle:
//
//	printf '[Result "*"]\n\n1. e4 e5 2. Nf3 (2. Bc4 Nc6) Nc6 *\n' | pgn-extract --quiet
func TestVariationForcesRenumber(t *testing.T) {
	g, r := gameAndReplay(t, "[Result \"*\"]\n\n1. e4 e5 2. Nf3 (2. Bc4 Nc6) Nc6 *\n")
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN})
	if err := w.WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	want := "[Event \"?\"]\n[Site \"?\"]\n[Date \"????.??.??\"]\n[Round \"?\"]\n[White \"?\"]\n[Black \"?\"]\n[Result \"*\"]\n\n1. e4 e5 2. Nf3 (2. Bc4 Nc6) 2... Nc6 *\n\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

// TestVariationStartsOnBlack pins that a variation attached to a black
// move (an alternative black reply) itself starts on black to move, i.e.
// its first move number is rendered "N..." not "N.". Oracle:
//
//	printf '[Result "*"]\n\n1. e4 e5 2. Nf3 Nc6 (2... Nf6 3. Nxe5 d6) 3. Bb5 a6 *\n' | pgn-extract --quiet
func TestVariationStartsOnBlack(t *testing.T) {
	src := "[Result \"*\"]\n\n1. e4 e5 2. Nf3 Nc6 (2... Nf6 3. Nxe5 d6) 3. Bb5 a6 *\n"
	g, r := gameAndReplay(t, src)
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN})
	if err := w.WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	want := "[Event \"?\"]\n[Site \"?\"]\n[Date \"????.??.??\"]\n[Round \"?\"]\n[White \"?\"]\n[Black \"?\"]\n[Result \"*\"]\n\n1. e4 e5 2. Nf3 Nc6 (2... Nf6 3. Nxe5 d6) 3. Bb5 a6 *\n\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

// TestSourceFormatNilReplay pins that Format == Source works with r == nil,
// using pgn.Move.SAN verbatim (as written) rather than normalized SAN.
func TestSourceFormatNilReplay(t *testing.T) {
	g, _ := gameAndReplay(t, fools)
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: Source})
	if err := w.WriteGame(g, nil); err != nil {
		t.Fatal(err)
	}
	want := `[Event "Casual"]
[Site "?"]
[Date "2020.01.01"]
[Round "1"]
[White "A"]
[Black "B"]
[Result "0-1"]

1. f3 e5 2. g4 { oops } 2... Qh4# 0-1

`
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

// TestNilReplayRequiresSource pins that WriteGame refuses a nil
// replay.Result for any format other than Source, erroring before
// anything is written.
func TestNilReplayRequiresSource(t *testing.T) {
	g, _ := gameAndReplay(t, fools)
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN})
	if err := w.WriteGame(g, nil); err == nil {
		t.Error("expected error for nil replay with Format SAN, got nil")
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output on error, got:\n%q", buf.String())
	}
}

// TestCommentWordWrap pins that long comments word-wrap: the comment text
// is split into individually wrappable words, and the black move after the
// wrapped comment still gets its "N..." renumber. Oracle:
//
//	printf '[Result "*"]\n\n1. e4 {this is a fairly long plain ascii comment that should wrap across several lines when width is small} e5 *\n' \
//	  | pgn-extract --quiet -w40
func TestCommentWordWrap(t *testing.T) {
	src := "[Result \"*\"]\n\n1. e4 {this is a fairly long plain ascii comment that should wrap across several lines when width is small} e5 *\n"
	g, r := gameAndReplay(t, src)
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN, LineWidth: 40})
	if err := w.WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	want := "[Event \"?\"]\n[Site \"?\"]\n[Date \"????.??.??\"]\n[Round \"?\"]\n[White \"?\"]\n[Black \"?\"]\n[Result \"*\"]\n\n" +
		"1. e4 { this is a fairly long plain\nascii comment that should wrap across\nseveral lines when width is small } 1...\ne5 *\n\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

// TestCommentWordWrapDefaultWidth pins the same word-wrapping at the
// default line width of 75. Oracle:
//
//	printf '[Result "*"]\n\n1. e4 {this is a fairly long plain ascii comment that should wrap across several lines when width is small at default width too maybe} e5 *\n' \
//	  | pgn-extract --quiet
func TestCommentWordWrapDefaultWidth(t *testing.T) {
	src := "[Result \"*\"]\n\n1. e4 {this is a fairly long plain ascii comment that should wrap across several lines when width is small at default width too maybe} e5 *\n"
	g, r := gameAndReplay(t, src)
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN})
	if err := w.WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	want := "[Event \"?\"]\n[Site \"?\"]\n[Date \"????.??.??\"]\n[Round \"?\"]\n[White \"?\"]\n[Black \"?\"]\n[Result \"*\"]\n\n" +
		"1. e4 { this is a fairly long plain ascii comment that should wrap across\nseveral lines when width is small at default width too maybe } 1... e5 *\n\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

// TestUnimplementedFormat pins that an out-of-range Format value (all nine
// defined formats are implemented) returns a clear, typed error rather than
// panicking or silently emitting nothing.
func TestUnimplementedFormat(t *testing.T) {
	g, r := gameAndReplay(t, "[Result \"*\"]\n\n1. e4 *\n")
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: Format(999)})
	if err := w.WriteGame(g, r); err == nil {
		t.Error("expected error for unimplemented format, got nil")
	}
}

func TestParseFormat(t *testing.T) {
	cases := map[string]Format{
		"san":    SAN,
		"source": Source,
		"lalg":   LAlg,
		"halg":   HAlg,
		"elalg":  ELAlg,
		"xlalg":  XLAlg,
		"uci":    UCI,
		"epd":    EPD,
		"fen":    FEN,
	}
	for s, want := range cases {
		got, err := ParseFormat(s)
		if err != nil {
			t.Errorf("ParseFormat(%q): unexpected error: %v", s, err)
			continue
		}
		if got != want {
			t.Errorf("ParseFormat(%q) = %v, want %v", s, got, want)
		}
	}
	if _, err := ParseFormat("bogus"); err == nil {
		t.Error("ParseFormat(bogus): expected error, got nil")
	}
}
