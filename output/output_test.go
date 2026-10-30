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

// TestRosterExtras pins that, under SevenTagRoster, RosterExtras tags the
// game carries are emitted after the seven in listed order (SubVariation is
// absent and so skipped), while other tags such as Annotator stay excluded.
// Oracle (mini.eco holds `[ECO "C00"][Opening "French"] 1. e4 e6 *`):
//
//	printf '[Event "E"]\n[Annotator "X"]\n[ECO "C00"]\n[Opening "French"]\n[Variation "Main"]\n[Result "*"]\n\n1. e4 e6 *\n' \
//	  | pgn-extract --quiet -7 -emini.eco
func TestRosterExtras(t *testing.T) {
	src := "[Event \"E\"]\n[Annotator \"X\"]\n[ECO \"C00\"]\n[Opening \"French\"]\n[Variation \"Main\"]\n[Result \"*\"]\n\n1. e4 e6 *\n"
	extras := []string{"ECO", "Opening", "Variation", "SubVariation"}

	g, r := gameAndReplay(t, src)
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN, SevenTagRoster: true, RosterExtras: extras})
	if err := w.WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	want := "[Event \"E\"]\n[Site \"?\"]\n[Date \"????.??.??\"]\n[Round \"?\"]\n[White \"?\"]\n[Black \"?\"]\n[Result \"*\"]\n[ECO \"C00\"]\n[Opening \"French\"]\n[Variation \"Main\"]\n\n1. e4 e6 *\n\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}

	// Without SevenTagRoster the extras list is inert: full tag output is
	// unchanged and Annotator is back.
	g, r = gameAndReplay(t, src)
	buf.Reset()
	w = NewWriter(&buf, Options{Format: SAN, RosterExtras: extras})
	if err := w.WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	want = "[Event \"E\"]\n[Site \"?\"]\n[Date \"????.??.??\"]\n[Round \"?\"]\n[White \"?\"]\n[Black \"?\"]\n[Result \"*\"]\n[ECO \"C00\"]\n[Opening \"French\"]\n[Variation \"Main\"]\n[Annotator \"X\"]\n\n1. e4 e6 *\n\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}

	// An explicit Roster still wins over both.
	g, r = gameAndReplay(t, src)
	buf.Reset()
	w = NewWriter(&buf, Options{Format: SAN, SevenTagRoster: true, RosterExtras: extras, Roster: []string{"Annotator"}})
	if err := w.WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	if want := "[Annotator \"X\"]\n\n1. e4 e6 *\n\n"; buf.String() != want {
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

// TestEmptyVsMissingRosterTags pins that a roster tag that is present but
// empty in the source is preserved as empty, while a genuinely missing tag
// gets the standard default — the oracle does NOT conflate the two. The
// Result tag is the exception: absent or empty, it is filled from the
// game's actual result (movetext terminator). Oracle:
//
//	printf '[Round ""]\n[Result "*"]\n\n1. e4 *\n' | pgn-extract --quiet
//	printf '[Result "*"]\n\n1. e4 *\n' | pgn-extract --quiet
//	printf '[Event ""]\n[Site ""]\n[Date ""]\n[Round ""]\n[White ""]\n[Black ""]\n[Result ""]\n\n1. e4 *\n' | pgn-extract --quiet
//	printf '[Result ""]\n\n1. e4 e5 1-0\n' | pgn-extract --quiet
func TestEmptyVsMissingRosterTags(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "empty Round preserved",
			src:  "[Round \"\"]\n[Result \"*\"]\n\n1. e4 *\n",
			want: "[Event \"?\"]\n[Site \"?\"]\n[Date \"????.??.??\"]\n[Round \"\"]\n[White \"?\"]\n[Black \"?\"]\n[Result \"*\"]\n\n1. e4 *\n\n",
		},
		{
			name: "missing Round defaults",
			src:  "[Result \"*\"]\n\n1. e4 *\n",
			want: "[Event \"?\"]\n[Site \"?\"]\n[Date \"????.??.??\"]\n[Round \"?\"]\n[White \"?\"]\n[Black \"?\"]\n[Result \"*\"]\n\n1. e4 *\n\n",
		},
		{
			name: "all empty preserved except Result",
			src:  "[Event \"\"]\n[Site \"\"]\n[Date \"\"]\n[Round \"\"]\n[White \"\"]\n[Black \"\"]\n[Result \"\"]\n\n1. e4 *\n",
			want: "[Event \"\"]\n[Site \"\"]\n[Date \"\"]\n[Round \"\"]\n[White \"\"]\n[Black \"\"]\n[Result \"*\"]\n\n1. e4 *\n\n",
		},
		{
			name: "empty Result takes game result",
			src:  "[Result \"\"]\n\n1. e4 e5 1-0\n",
			want: "[Event \"?\"]\n[Site \"?\"]\n[Date \"????.??.??\"]\n[Round \"?\"]\n[White \"?\"]\n[Black \"?\"]\n[Result \"1-0\"]\n\n1. e4 e5 1-0\n\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, r := gameAndReplay(t, tc.src)
			var buf bytes.Buffer
			w := NewWriter(&buf, Options{Format: SAN})
			if err := w.WriteGame(g, r); err != nil {
				t.Fatal(err)
			}
			if buf.String() != tc.want {
				t.Errorf("got:\n%q\nwant:\n%q", buf.String(), tc.want)
			}
		})
	}
}

// TestPrefixCommentSpacing pins that game-level prefix comments are
// whitespace-normalized (single spaces inside "{ text }"), word-wrapped at
// the configured line width, and joined into one token stream when there
// are several — exactly like the C oracle. Oracle:
//
//	printf '[Result "*"]\n\n{  padded   internal    spacing  }\n\n1. e4 *\n' | pgn-extract --quiet
//	printf '[Result "*"]\n\n{this is a fairly long game level prefix comment that should wrap across lines}\n\n1. e4 *\n' | pgn-extract --quiet -w40
//	printf '[Result "*"]\n\n{first comment}\n{second comment}\n\n1. e4 *\n' | pgn-extract --quiet
func TestPrefixCommentSpacing(t *testing.T) {
	const tags = "[Event \"?\"]\n[Site \"?\"]\n[Date \"????.??.??\"]\n[Round \"?\"]\n[White \"?\"]\n[Black \"?\"]\n[Result \"*\"]\n\n"
	cases := []struct {
		name  string
		src   string
		width int
		want  string
	}{
		{
			name: "internal padding normalized",
			src:  "[Result \"*\"]\n\n{  padded   internal    spacing  }\n\n1. e4 *\n",
			want: tags + "{ padded internal spacing }\n\n1. e4 *\n\n",
		},
		{
			name:  "long prefix comment wraps",
			src:   "[Result \"*\"]\n\n{this is a fairly long game level prefix comment that should wrap across lines}\n\n1. e4 *\n",
			width: 40,
			want:  tags + "{ this is a fairly long game level\nprefix comment that should wrap across\nlines }\n\n1. e4 *\n\n",
		},
		{
			name: "multiple prefix comments share a line",
			src:  "[Result \"*\"]\n\n{first comment}\n{second comment}\n\n1. e4 *\n",
			want: tags + "{ first comment } { second comment }\n\n1. e4 *\n\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, r := gameAndReplay(t, tc.src)
			var buf bytes.Buffer
			w := NewWriter(&buf, Options{Format: SAN, LineWidth: tc.width})
			if err := w.WriteGame(g, r); err != nil {
				t.Fatal(err)
			}
			if buf.String() != tc.want {
				t.Errorf("got:\n%q\nwant:\n%q", buf.String(), tc.want)
			}
		})
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

// TestMoveNumberFromFEN pins that a game starting from a FEN tag with a
// nonzero fullmove field (an endgame study, not a game from the start
// position) numbers its first move from that field rather than always
// starting at 1. Oracle:
//
//	printf '[FEN "7k/3P4/7K/8/8/8/8/8 w - - 6 55"]\n[SetUp "1"]\n[Result "*"]\n\nd8=Q *\n' | pgn-extract --quiet
func TestMoveNumberFromFEN(t *testing.T) {
	src := "[FEN \"7k/3P4/7K/8/8/8/8/8 w - - 6 55\"]\n[SetUp \"1\"]\n[Result \"*\"]\n\nd8=Q *\n"
	g, r := gameAndReplay(t, src)
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN})
	if err := w.WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "55. d8=Q") {
		t.Errorf("got:\n%q\nwant a line containing \"55. d8=Q\"", buf.String())
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

func TestUnknownTagsRunWideFirstSeenOrder(t *testing.T) {
	// pgn-extract emits unrecognised tags by tag index, assigned on first
	// sight anywhere in the run; a later game listing them in a different
	// order still comes out in the first game's order.
	g1 := parseGame(t, "[Event \"e\"]\n[Alpha \"1\"]\n[Beta \"2\"]\n\n*\n")
	g2 := parseGame(t, "[Event \"e\"]\n[Beta \"2\"]\n[Gamma \"3\"]\n[Alpha \"1\"]\n\n*\n")
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: Source})
	for _, g := range []*pgn.Game{g1, g2} {
		if err := w.WriteGame(g, nil); err != nil {
			t.Fatal(err)
		}
	}
	out := buf.String()
	second := out[strings.LastIndex(out, "[Event"):]
	a, b, c := strings.Index(second, "[Alpha"), strings.Index(second, "[Beta"), strings.Index(second, "[Gamma")
	if !(a < b && b < c) {
		t.Errorf("want Alpha, Beta, Gamma order in second game:\n%s", second)
	}

	// A shared registry seeded by Observe orders tags of a game that was
	// never written.
	reg := NewTagRegistry()
	reg.Observe(parseGame(t, "[Zeta \"0\"]\n[Alpha \"1\"]\n\n*\n"))
	buf.Reset()
	w = NewWriter(&buf, Options{Format: Source, Tags: reg})
	w.WriteGame(parseGame(t, "[Alpha \"1\"]\n[Zeta \"0\"]\n\n*\n"), nil)
	if z, a := strings.Index(buf.String(), "[Zeta"), strings.Index(buf.String(), "[Alpha"); z > a {
		t.Errorf("registry order not applied:\n%s", buf.String())
	}

	// Predefined-but-unordered oracle tags precede unknown ones.
	buf.Reset()
	w = NewWriter(&buf, Options{Format: Source})
	w.WriteGame(parseGame(t, "[Unknown \"u\"]\n[HashCode \"abc\"]\n[BlackFideId \"1\"]\n\n*\n"), nil)
	s := buf.String()
	if !(strings.Index(s, "[BlackFideId") < strings.Index(s, "[HashCode") && strings.Index(s, "[HashCode") < strings.Index(s, "[Unknown")) {
		t.Errorf("predefined trailing tags order:\n%s", s)
	}
}

// parseGame parses src as a single PGN game without replaying it.
func parseGame(t *testing.T, src string) *pgn.Game {
	t.Helper()
	s := pgn.NewScanner(strings.NewReader(src), pgn.ParserOptions{})
	if !s.Next() {
		t.Fatalf("no game parsed: %v", s.Err())
	}
	return s.Game()
}

func TestResultTagNormalisation(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"[Result \"1/2\"]\n\n1. e4 1/2-1/2\n", "1/2-1/2"},
		{"[Result \"1/2\"]\n\n1. e4 *\n", "1/2-1/2"},
		{"[Result \"?\"]\n\n1. e4 1-0\n", "1-0"},
		{"[Result \"\"]\n\n1. e4 0-1\n", "0-1"},
		{"1. e4 0-1\n", "0-1"},
		{"[Result \"1:0\"]\n\n1. e4 1-0\n", "*"},
		{"[Result \"1-0\"]\n\n1. e4 0-1\n", "1-0"}, // disagreement is filter's business
		{"[Event \"x\"]\n\n*\n", "*"},
	}
	for _, c := range cases {
		g := parseGame(t, c.src)
		var buf bytes.Buffer
		if err := NewWriter(&buf, Options{Format: Source}).WriteGame(g, nil); err != nil {
			t.Fatal(err)
		}
		want := "[Result \"" + c.want + "\"]\n"
		if !strings.Contains(buf.String(), want) {
			t.Errorf("%q: want %s in\n%s", c.src, want, buf.String())
		}
	}
}
