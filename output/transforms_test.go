package output

import (
	"bytes"
	"strings"
	"testing"
)

// movetext returns the part of a rendered game after the tag section.
func movetext(t *testing.T, out string) string {
	t.Helper()
	_, rest, ok := strings.Cut(out, "\n\n")
	if !ok {
		t.Fatalf("no tag section in %q", out)
	}
	return rest
}

const annotated = "[Result \"*\"]\n\n{pre} 1. e4 {c1} e5 $1 {c2} (1... c5 {alt} 2. Nf3) 2. Nf3 Nc6 *\n"

// Every expected string in this file is the C oracle's output for the
// same input and the flag named on the test:
//
//	printf '...' | pgn-extract --<flag> --quiet

func TestEvaluation(t *testing.T) {
	got := movetext(t, render(t, fools, Options{Evaluation: true, NoComments: true}))
	want := "1. f3 { -0.10 } 1... e5 { -1.10 } 2. g4 { -1.00 } 2... Qh4# { -3.90 } 0-1\n\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	got = movetext(t, render(t, annotated, Options{Evaluation: true}))
	want = "{ pre }\n\n1. e4 { c1 } { 1.00 } 1... e5 $1 { c2 } { 0.00 } (1... c5 { alt } { 0.80 }\n2. Nf3 { 0.60 }) 2. Nf3 { -0.20 } 2... Nc6 { -0.40 } *\n\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestHashComments(t *testing.T) {
	got := movetext(t, render(t, annotated, Options{HashComments: true}))
	want := "{ pre }\n\n1. e4 { c1 } { 823c9b50fd114196 } 1... e5 $1 { c2 } { 0844931a6ef4b9a0 } (\n1... c5 { alt } { 644d4afe02564aeb } 2. Nf3 { bf29a6086ab02bd6 }) 2. Nf3 {\nd3207fec0612d89d } 2... Nc6 { 78cda70e17837d9e } *\n\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFENComments(t *testing.T) {
	got := movetext(t, render(t, annotated, Options{FENComments: true}))
	want := "KQkq e6 0 2 } (1... c5 { alt } {\nrnbqkbnr/pp1ppppp/8/2p5/4P3/8/PPPP1PPP/RNBQKBNR w KQkq c6 0 2 } 2. Nf3 {\nrnbqkbnr/pp1ppppp/8/2p5/4P3/5N2/PPPP1PPP/RNBQKB1R b KQkq - 1 2 }) 2. Nf3 {\nrnbqkbnr/pppp1ppp/8/4p3/4P3/5N2/PPPP1PPP/RNBQKB1R b KQkq - 1 2 } 2... Nc6 {\nr1bqkbnr/pppp1ppp/2n5/4p3/4P3/5N2/PPPP1PPP/RNBQKB1R w KQkq - 2 3 } *\n\n"
	if !strings.HasSuffix(got, want) {
		t.Errorf("got:\n%s\nwant suffix:\n%s", got, want)
	}
	// Generated comments follow the move's own annotations and precede
	// its variations, in the order evaluation, FEN, hash; -F is implied
	// off by --fencomments.
	got = movetext(t, render(t, "[Result \"*\"]\n\n1. e4 $1 {c} (1. d4) e5 *\n",
		Options{FENComments: true, HashComments: true, Evaluation: true, FinalFEN: true}))
	want = "830eb9b20758d1de }) 1... e5 { 0.00 } {\nrnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR w KQkq e6 0 2 } {\n0844931a6ef4b9a0 } *\n\n"
	if !strings.HasSuffix(got, want) {
		t.Errorf("got:\n%s\nwant suffix:\n%s", got, want)
	}
}

func TestFinalFEN(t *testing.T) {
	got := movetext(t, render(t, annotated, Options{FinalFEN: true}))
	want := "{ pre }\n\n1. e4 { c1 } 1... e5 $1 { c2 } (1... c5 { alt } 2. Nf3) 2. Nf3 Nc6 {\n\"r1bqkbnr/pppp1ppp/2n5/4p3/4P3/5N2/PPPP1PPP/RNBQKB1R w KQkq - 2 3\" } *\n\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	// Not subject to --nocomments or --noresults; absent from a zero-move game.
	got = movetext(t, render(t, "[Result \"1-0\"]\n\n1. e4 e5 {x} 1-0\n", Options{FinalFEN: true, NoComments: true, NoResults: true}))
	want = "1. e4 e5 { \"rnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR w KQkq e6 0 2\"\n}\n\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	got = movetext(t, render(t, "[Result \"*\"]\n\n*\n", Options{FinalFEN: true}))
	if got != "*\n\n" {
		t.Errorf("zero-move game: got %q, want \"*\\n\\n\"", got)
	}
}

func TestCommentLines(t *testing.T) {
	got := movetext(t, render(t, annotated, Options{CommentLines: true}))
	want := "{ pre }\n\n1. e4\n{ c1 }\n1... e5 $1\n{ c2 }\n(1... c5\n{ alt }\n2. Nf3) 2. Nf3 Nc6 *\n\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	got = movetext(t, render(t, "[Result \"1-0\"]\n\n1. d4 ({pc} 1. e4) d5 {x} 1-0\n", Options{CommentLines: true}))
	want = "1. d4 (\n{ pc }\n1. e4) 1... d5\n{ x }\n1-0\n\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	got = movetext(t, render(t, "[Result \"*\"]\n\n{a} {b} 1. e4 *\n", Options{CommentLines: true}))
	want = "{ a }\n{ b }\n\n1. e4 *\n\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestNoTags(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{NoTags: true})
	for _, src := range []string{
		"[Event \"x\"]\n[Result \"*\"]\n\n1. e4 e5 *\n",
		"[Event \"y\"]\n[Result \"1-0\"]\n\n{pre} 1. d4 d5 1-0\n",
		"[Event \"z\"]\n[Result \"*\"]\n\n*\n",
	} {
		g, r := gameAndReplay(t, src)
		if err := w.WriteGame(g, r); err != nil {
			t.Fatal(err)
		}
	}
	want := "1. e4 e5 *\n\n{ pre }\n\n1. d4 d5 1-0\n\n*\n\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}

	got := render(t, "[Event \"x\"]\n[Result \"*\"]\n\n1. e4 *\n", Options{NoTags: true, Format: FEN})
	want = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1\nrnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1\n\n"
	if got != want {
		t.Errorf("fen: got:\n%q\nwant:\n%q", got, want)
	}
	got = render(t, "[Event \"x\"]\n[Result \"*\"]\n\n1. e4 *\n", Options{NoTags: true, Format: EPD})
	want = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - \nrnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 \n\n"
	if got != want {
		t.Errorf("epd: got:\n%q\nwant:\n%q", got, want)
	}
}

func TestNoChecks(t *testing.T) {
	got := movetext(t, render(t, fools, Options{NoChecks: true}))
	if want := "1. f3 e5 2. g4 { oops } 2... Qh4 0-1\n\n"; got != want {
		t.Errorf("san: got %q, want %q", got, want)
	}
	got = movetext(t, render(t, fools, Options{NoChecks: true, Format: ELAlg}))
	if want := "1. f2f3 e7e5 2. g2g4 { oops } 2... Qd8h4 0-1\n\n"; got != want {
		t.Errorf("elalg: got %q, want %q", got, want)
	}
	got = movetext(t, render(t, "[Result \"*\"]\n[FEN \"k7/4P3/8/8/8/8/8/K7 w - - 0 1\"]\n\n1. e8=Q+ Ka7 2. Qb8+ *\n", Options{NoChecks: true}))
	if want := "1. e8=Q Ka7 2. Qb8 *\n\n"; got != want {
		t.Errorf("promotion: got %q, want %q", got, want)
	}
	got = movetext(t, render(t, fools, Options{NoChecks: true, Format: Source}))
	if want := "1. f3 e5 2. g4 { oops } 2... Qh4 0-1\n\n"; got != want {
		t.Errorf("source: got %q, want %q", got, want)
	}
}

func TestPlyLimit(t *testing.T) {
	cases := []struct {
		name string
		src  string
		opts Options
		want string
	}{
		{"3", annotated, Options{HasPlyLimit: true, PlyLimit: 3},
			"{ pre }\n\n1. e4 { c1 } 1... e5 $1 { c2 } (1... c5 { alt } 2. Nf3) 2. Nf3 *\n\n"},
		{"0", annotated, Options{HasPlyLimit: true, PlyLimit: 0}, "{ pre }\n\n*\n\n"},
		{"2-noresults", annotated, Options{HasPlyLimit: true, PlyLimit: 2, NoResults: true},
			"{ pre }\n\n1. e4 { c1 } 1... e5 $1 { c2 } (1... c5 { alt })\n\n"},
		{"whole-game", "[Result \"1-0\"]\n\n1. d4 d5 1-0\n", Options{HasPlyLimit: true, PlyLimit: 3}, "1. d4 d5 1-0\n\n"},
		{"truncated-result-is-star", "[Result \"1-0\"]\n\n1. d4 d5 1-0\n", Options{HasPlyLimit: true, PlyLimit: 1}, "1. d4 *\n\n"},
		// The limit is an absolute ply number: a game starting at ply 2 (black to move) is past limit 1.
		{"fen-start", "[Result \"1/2-1/2\"]\n[SetUp \"1\"]\n[FEN \"rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq - 0 1\"]\n\n1... e5 2. Nf3 {cmt} Nc6 3. Bb5 1/2-1/2\n",
			Options{HasPlyLimit: true, PlyLimit: 1}, "*\n\n"},
		{"fen-start-3", "[Result \"1/2-1/2\"]\n[SetUp \"1\"]\n[FEN \"rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq - 0 1\"]\n\n1... e5 2. Nf3 {cmt} Nc6 3. Bb5 1/2-1/2\n",
			Options{HasPlyLimit: true, PlyLimit: 3}, "1... e5 2. Nf3 { cmt } *\n\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := movetext(t, render(t, c.src, c.opts))
			if got != c.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}

	// FEN output honours the limit only at the start (output.c
	// print_FEN_move_list): past it nothing is listed, within it every
	// position is.
	got := render(t, "[Result \"*\"]\n\n1. e4 e5 2. Nf3 *\n", Options{Format: FEN, HasPlyLimit: true, PlyLimit: 2})
	if !strings.HasSuffix(got, "\n\nrnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1\nrnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1\nrnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR w KQkq e6 0 2\nrnbqkbnr/pppp1ppp/8/4p3/4P3/5N2/PPPP1PPP/RNBQKB1R b KQkq - 1 2\n\n") {
		t.Errorf("fen limit 2: got %q", got)
	}
	got = render(t, "[Result \"*\"]\n\n1. e4 e5 2. Nf3 *\n", Options{Format: FEN, HasPlyLimit: true, PlyLimit: 0})
	if !strings.HasSuffix(got, "[Result \"*\"]\n\n\n") {
		t.Errorf("fen limit 0: got %q", got)
	}
}
