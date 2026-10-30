package eco

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyleboon/pgnx/chess"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// goldenDir holds the C repo's test corpus plus its eco.pgn.
const goldenDir = "../cmd/pgnx/testdata/golden"

// smallECO is an inline ECO file exercising every matching rule.
const smallECO = `[ECO "A00"]
[Opening "Polish (Sokolsky) opening"]

1. b4 *

[ECO "A00"]
[Opening "Polish"]
[Variation "Tuebingen variation"]

1. b4 Nh6 *

[ECO "A04"]
[Opening "Reti opening"]

1. Nf3 *

[ECO "T01"]
[Opening "First order"]

1. e4 e5 2. Nf3 Nc6 *

[ECO "T02"]
[Opening "Second order"]

1. Nf3 Nc6 2. e4 e5 *

[ECO "T03"]
[Opening "Longer sibling"]
[Variation "Ruy Lopez"]
[SubVariation "Morphy defence"]

1. e4 e5 2. Nf3 Nc6 3. Bb5 a6 *

[ECO "X1"]
[Opening "Queen's pawn"]

1. d4 *

[ECO "X2"]
[Opening "Duplicate of X1"]

1. d4 *

[ECO "B20"]
[Opening "Sicilian from a FEN"]
[FEN "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq - 0 1"]

1... c5 *

[ECO "ZZ"]
[Opening "Broken line"]

1. e4 e4 *
`

func loadSmall(t *testing.T) *Table {
	t.Helper()
	tab, err := Load(strings.NewReader(smallECO))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return tab
}

func parseGame(t *testing.T, src string) *pgn.Game {
	t.Helper()
	sc := pgn.NewScanner(strings.NewReader(src), pgn.ParserOptions{})
	if !sc.Next() {
		t.Fatalf("no game in %q: %v", src, sc.Err())
	}
	return sc.Game()
}

func replayed(t *testing.T, src string) *replay.Result {
	t.Helper()
	res, err := replay.Game(parseGame(t, src), replay.Options{})
	if err != nil {
		t.Fatalf("replay %q: %v", src, err)
	}
	return res
}

func TestLoadSmall(t *testing.T) {
	tab := loadSmall(t)
	if tab.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (the illegal 1. e4 e4 line)", tab.Skipped)
	}
	// Ten lines minus the broken one minus the X2 duplicate of X1.
	if tab.Len() != 8 {
		t.Errorf("Len = %d, want 8", tab.Len())
	}
	// Longest line is six plies: 1. e4 e5 2. Nf3 Nc6 3. Bb5 a6.
	if tab.maxHalfMoves != 12 {
		t.Errorf("maxHalfMoves = %d, want 12", tab.maxHalfMoves)
	}
}

func TestClassify(t *testing.T) {
	tab := loadSmall(t)
	cases := []struct {
		name        string
		game        string
		wantECO     string
		wantOpening string
		wantOk      bool
	}{
		{"exact at last ply", "1. b4 Nh6 *", "A00", "Polish", true},
		{"earlier ply match survives an unmatched later ply", "1. b4 Nf6 *", "A00", "Polish (Sokolsky) opening", true},
		{"exact beats a partial reaching the same placement", "1. Nf3 Nc6 2. e4 e5 *", "T02", "Second order", true},
		{"third move order: earliest-loaded partial wins", "1. e4 Nc6 2. Nf3 e5 *", "T01", "First order", true},
		{"duplicate line keeps the first entry", "1. d4 *", "X1", "Queen's pawn", true},
		{"FEN start honoured; partial with different cumulative", "1. e4 c5 *", "B20", "Sicilian from a FEN", true},
		{"latest ply wins", "1. e4 e5 2. Nf3 Nc6 3. Bb5 a6 *", "T03", "Longer sibling", true},
		{"seven plies past the longest line keeps the last in-window match",
			"1. e4 e5 2. Nf3 Nc6 3. Bb5 a6 4. Ba4 Nf6 5. O-O Be7 6. Re1 b5 7. Bb3 *", "T03", "Longer sibling", true},
		{"no moves", "*", "", "", false},
		{"nothing matches", "1. h4 *", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, ok := tab.Classify(replayed(t, c.game))
			if ok != c.wantOk {
				t.Fatalf("Classify ok = %v, want %v (entry %+v)", ok, c.wantOk, e)
			}
			if !ok {
				return
			}
			if e.ECO != c.wantECO || e.Opening != c.wantOpening {
				t.Errorf("Classify = %+v, want ECO %q Opening %q", e, c.wantECO, c.wantOpening)
			}
		})
	}
}

func TestClassifyNilTable(t *testing.T) {
	var tab *Table
	if _, ok := tab.Classify(replayed(t, "1. e4 *")); ok {
		t.Error("nil table matched")
	}
	if _, ok := loadSmall(t).Classify(nil); ok {
		t.Error("nil result matched")
	}
}

// TestMatchWindow pins eco_matches' partial-match rules on a single
// position: same side to move, 0 <= ply difference <= 6.
func TestMatchWindow(t *testing.T) {
	tab := loadSmall(t)
	res := replayed(t, "1. e4 e5 2. Nf3 Nc6 *")
	final := res.Positions[len(res.Positions)-1]
	key := final.Placement
	const unrelatedCum = 12345 // never equals a stored cumulative
	cases := []struct {
		half   int
		toMove chess.Color
		want   string // "" for no match
	}{
		{4, chess.White, "T01"},  // diff 0, partial (cumulative differs)
		{10, chess.White, "T01"}, // diff 6, edge of the window
		{11, chess.White, ""},    // diff 7, too distant
		{3, chess.White, ""},     // line longer than the game position
		{4, chess.Black, ""},     // wrong side to move
	}
	for _, c := range cases {
		e := tab.match(key, c.half, c.toMove, unrelatedCum)
		got := ""
		if e != nil {
			got = e.ECO
		}
		if got != c.want {
			t.Errorf("match(half=%d, toMove=%v) = %q, want %q", c.half, c.toMove, got, c.want)
		}
	}
}

func TestApply(t *testing.T) {
	g := parseGame(t, `[Event "Test"]
[ECO "B99"]
[Opening "Stale opening"]
[Site "Here"]
[Variation "Stale variation"]
[SubVariation "Stale sub"]

1. b4 *
`)
	tab := loadSmall(t)
	e, ok := tab.Classify(replayed(t, "1. b4 *"))
	if !ok {
		t.Fatal("no match for 1. b4")
	}
	Apply(g, e)
	want := []pgn.Tag{
		{Name: "Event", Value: "Test"},
		{Name: "Site", Value: "Here"},
		{Name: "ECO", Value: "A00"},
		{Name: "Opening", Value: "Polish (Sokolsky) opening"},
	}
	if !equalTags(g.Tags, want) {
		t.Errorf("Apply (entry without Variation) tags = %v, want %v", g.Tags, want)
	}

	e, ok = tab.Classify(replayed(t, "1. e4 e5 2. Nf3 Nc6 3. Bb5 a6 *"))
	if !ok {
		t.Fatal("no match for the Ruy Lopez line")
	}
	Apply(g, e)
	want = []pgn.Tag{
		{Name: "Event", Value: "Test"},
		{Name: "Site", Value: "Here"},
		{Name: "ECO", Value: "T03"},
		{Name: "Opening", Value: "Longer sibling"},
		{Name: "Variation", Value: "Ruy Lopez"},
		{Name: "SubVariation", Value: "Morphy defence"},
	}
	if !equalTags(g.Tags, want) {
		t.Errorf("Apply (full entry) tags = %v, want %v", g.Tags, want)
	}

	Apply(nil, e) // must not panic
}

func equalTags(a, b []pgn.Tag) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFilePrefix(t *testing.T) {
	cases := []struct {
		eco   string
		level int
		want  string
	}{
		{"A13", 1, "A"},
		{"A13", 2, "A1"},
		{"A13", 3, "A13"},
		{"A13", 5, "A13"},
		{"", 1, "noeco"},
		{"?", 1, "noeco"},
		{"1A", 1, "noeco"},
		{"b14", 2, "b1"},
		{"A13", 0, "noeco"},
	}
	for _, c := range cases {
		if got := FilePrefix(c.eco, c.level); got != c.want {
			t.Errorf("FilePrefix(%q, %d) = %q, want %q", c.eco, c.level, got, c.want)
		}
	}
}

func openGolden(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open(filepath.Join(goldenDir, name))
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// TestLoadVendoredECO loads the C repo's eco.pgn: 2014 lines, none
// broken, and (as the C tool logs no "ECO hash collision") none skipped
// as duplicates.
func TestLoadVendoredECO(t *testing.T) {
	tab, err := Load(openGolden(t, "eco.pgn"))
	if err != nil {
		t.Fatalf("Load eco.pgn: %v", err)
	}
	if tab.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", tab.Skipped)
	}
	if tab.Len() != 2014 {
		t.Errorf("Len = %d, want 2014", tab.Len())
	}
}

// TestClassifyGoldenTestE classifies the games of the C repo's test-e
// target (pgn-extract -e eco.pgn test-e.pgn) and compares the four ECO
// tags with the oracle output test-e-out.pgn game by game.
func TestClassifyGoldenTestE(t *testing.T) {
	tab, err := Load(openGolden(t, "eco.pgn"))
	if err != nil {
		t.Fatalf("Load eco.pgn: %v", err)
	}
	in := pgn.NewScanner(openGolden(t, "infiles/test-e.pgn"), pgn.ParserOptions{})
	out := pgn.NewScanner(openGolden(t, "outfiles/test-e-out.pgn"), pgn.ParserOptions{})
	n := 0
	for in.Next() {
		if !out.Next() {
			t.Fatalf("oracle has fewer games than the input (%d)", n)
		}
		n++
		g := in.Game()
		res, err := replay.Game(g, replay.Options{})
		if err != nil {
			t.Fatalf("game %d: replay: %v", n, err)
		}
		if e, ok := tab.Classify(res); ok {
			Apply(g, e)
		}
		for _, name := range tagNames {
			got, gotOK := g.Tag(name)
			want, wantOK := out.Game().Tag(name)
			if got != want || gotOK != wantOK {
				t.Errorf("game %d: %s = %q (present %v), oracle %q (present %v)", n, name, got, gotOK, want, wantOK)
			}
		}
	}
	if err := in.Err(); err != nil {
		t.Fatal(err)
	}
	if out.Next() {
		t.Errorf("oracle has more games than the input (%d)", n)
	}
	if n == 0 {
		t.Fatal("no games read")
	}
}
