package transform

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kyleboon/pgnx/output"
	"github.com/kyleboon/pgnx/pgn"
)

// render returns g in Source format, tags and movetext, as the writer
// emits it: the shape the C oracle's split output is compared in.
func render(t *testing.T, g *pgn.Game) string {
	t.Helper()
	var buf bytes.Buffer
	w := output.NewWriter(&buf, output.Options{Format: output.Source, Roster: []string{"Event", "Result"}})
	if err := w.WriteGame(g, nil); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func splitTexts(t *testing.T, src string, depth int) []string {
	t.Helper()
	g, _ := parse(t, src)
	var texts []string
	for _, sg := range SplitVariations(g, depth) {
		texts = append(texts, render(t, sg))
	}
	return texts
}

func checkSplit(t *testing.T, src string, depth int, want []string) {
	t.Helper()
	got := splitTexts(t, src, depth)
	if len(got) != len(want) {
		t.Fatalf("%d games, want %d:\n%s", len(got), len(want), strings.Join(got, ""))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("game %d:\n--- got ---\n%s--- want ---\n%s", i, got[i], want[i])
		}
	}
}

// The C test suite's test-splitvariants target:
//
//	pgn-extract --splitvariants -otest-splitvariants-out.pgn --quiet test-splitvariants.pgn
//
// The main line comes first, stripped of its variations, with the comment
// that followed a variation's closing paren moved onto the move before
// it; then one game per variation, depth first, each carrying the
// variation's prefix comment on the move before the branch, a Result tag
// of "*" and a "*" result token.
func TestSplitVariationsGolden(t *testing.T) {
	const src = "[Event \"?\"]\n[Result \"*\"]\n\n" +
		"1. e4 { comment1 } e5 { comment2 }\n" +
		"( { comment3 } 1... e6 (1... d6) 2. d4 (2. Nf3){ comment4 } ) { comment5 } 2. Nf3\n*\n"
	const tags = "[Event \"?\"]\n[Result \"*\"]\n\n"
	checkSplit(t, src, 0, []string{
		tags + "1. e4 { comment1 } 1... e5 { comment2 } { comment5 } 2. Nf3 *\n\n",
		tags + "1. e4 { comment1 } { comment3 } 1... e6 2. d4 { comment4 } *\n\n",
		tags + "1. e4 { comment1 } { comment3 } 1... d6 *\n\n",
		tags + "1. e4 { comment1 } { comment3 } 1... e6 2. Nf3 *\n\n",
	})
}

// Every expected text below is the C oracle's:
//
//	pgn-extract --splitvariants --quiet
const splitTags = "[Event \"\"]\n[Result \"*\"]\n\n"

// The comments that followed variations go after the move's leading
// comments and before its NAGs, in order.
func TestSplitVariationsGathersSuffixCommentsBeforeNAGs(t *testing.T) {
	checkSplit(t, "[Result \"*\"]\n\n1. e4 e5 { c2 } $1 { n } (1... e6) { c5 } (1... d6) { c6 } 2. Nf3 *\n", 0, []string{
		splitTags + "1. e4 e5 { c2 } { c5 } { c6 } $1 { n } 2. Nf3 *\n\n",
		splitTags + "1. e4 e6 *\n\n",
		splitTags + "1. e4 d6 *\n\n",
	})
}

// A variation replacing the first move puts its prefix comment after the
// game's.
func TestSplitVariationsFirstMovePrefixComment(t *testing.T) {
	checkSplit(t, "[Result \"*\"]\n\n{ pre } 1. e4 ({ vp } 1. d4 d5) { s0 } e5 *\n", 0, []string{
		splitTags + "{ pre }\n\n1. e4 { s0 } 1... e5 *\n\n",
		splitTags + "{ pre } { vp }\n\n1. d4 d5 *\n\n",
	})
}

// A nested variation's game is emitted right after its parent's, with
// the parent's own suffix comments gathered.
func TestSplitVariationsNested(t *testing.T) {
	checkSplit(t, "[Result \"*\"]\n\n1. e4 e5 2. Nf3 (2. Nc3 { x } (2. f4) { y } 2... Nf6) Nc6 *\n", 0, []string{
		splitTags + "1. e4 e5 2. Nf3 Nc6 *\n\n",
		splitTags + "1. e4 e5 2. Nc3 { x } { y } 2... Nf6 *\n\n",
		splitTags + "1. e4 e5 2. f4 *\n\n",
	})
}

// A variation's game keeps the variation's own result token under a
// Result tag of "*"; the main line keeps both of its own.
func TestSplitVariationsResult(t *testing.T) {
	checkSplit(t, "[Result \"1-0\"]\n\n1. e4 e5 (1... c5 0-1) 2. Nf3 1-0\n", 0, []string{
		"[Event \"\"]\n[Result \"1-0\"]\n\n1. e4 e5 2. Nf3 1-0\n\n",
		splitTags + "1. e4 c5 0-1\n\n",
	})
}

// --splitvariants N stops after N levels of variations.
func TestSplitVariationsDepth(t *testing.T) {
	const src = "[Result \"1-0\"]\n\n1. e4 e5 (1... e6 (1... d6) 2. d4 (2. Nf3)) 2. Nf3 Nc6 (2... Nf6 3. Nxe5 (3. Nc3)) 3. Bb5 1-0\n"
	checkSplit(t, src, 1, []string{
		"[Event \"\"]\n[Result \"1-0\"]\n\n1. e4 e5 2. Nf3 Nc6 3. Bb5 1-0\n\n",
		splitTags + "1. e4 e6 2. d4 *\n\n",
		splitTags + "1. e4 e5 2. Nf3 Nf6 3. Nxe5 *\n\n",
	})
	if n := len(splitTexts(t, src, 0)); n != 6 {
		t.Errorf("unlimited depth: %d games, want 6", n)
	}
	if n := len(splitTexts(t, src, 2)); n != 6 {
		t.Errorf("depth 2: %d games, want 6", n)
	}
}

// The source game is not touched: the split games are copies.
func TestSplitVariationsLeavesSourceAlone(t *testing.T) {
	const src = "[Result \"*\"]\n\n{ pre } 1. e4 ({ vp } 1. d4) { s0 } e5 { c } (1... c5) $1 *\n"
	g, _ := parse(t, src)
	before := render(t, g)
	games := SplitVariations(g, 0)
	games[0].Moves[0].AddComment("x")
	games[1].PrefixComments = append(games[1].PrefixComments, "y")
	games[1].SetTag("Result", "1-0")
	if after := render(t, g); after != before {
		t.Errorf("source game changed:\n--- before ---\n%s--- after ---\n%s", before, after)
	}
}

// An empty variation emits no game (split_variants skips a variation
// with no moves), and its prefix comment goes with it.
func TestSplitVariationsSkipsEmptyVariation(t *testing.T) {
	checkSplit(t, "[Result \"*\"]\n\n1. e4 ({ pre }) e5 (1... c5) *\n", 0, []string{
		splitTags + "1. e4 e5 *\n\n",
		splitTags + "1. e4 c5 *\n\n",
	})
}
