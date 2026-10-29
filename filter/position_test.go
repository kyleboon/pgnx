package filter

import (
	"errors"
	"strings"
	"testing"

	"github.com/kyleboon/pgnx/chess"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

const (
	najdorfLine = "e4 c5 Nf3 d6 d4 cxd4 Nxd4 Nf6 Nc3 a6 Be3"
	najdorfGame = "1. e4 c5 2. Nf3 d6 3. d4 cxd4 4. Nxd4 Nf6 5. Nc3 a6 6. Be3 e6 7. f3 *\n"
	// The Be3 position reached at ply 25 instead of 11: queen shuffles
	// that return to the position after 5... a6 before 13. Be3.
	najdorfTransposed = "1. e4 c5 2. Nf3 d6 3. d4 cxd4 4. Nxd4 Nf6 5. Nc3 a6 " +
		"6. Qd2 Qc7 7. Qd1 Qd8 8. Qd2 Qc7 9. Qd1 Qd8 " +
		"10. Qd2 Qc7 11. Qd3 Qb6 12. Qd1 Qd8 13. Be3 e6 14. f3 *\n"
	initialFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"
)

func lineCriteria(t *testing.T, line string) *PositionCriteria {
	t.Helper()
	pc := &PositionCriteria{}
	if err := pc.AddLine(line); err != nil {
		t.Fatalf("AddLine(%q): %v", line, err)
	}
	return pc
}

// commentsAt returns the index of every main-line move carrying a comment.
func commentsAt(g *pgn.Game) []int {
	var out []int
	for i, m := range g.Moves {
		if len(m.Comments) > 0 {
			out = append(out, i)
		}
	}
	return out
}

func TestPositionalLineMoves(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"e4 c5 Nf3", "e4 c5 Nf3"},
		{"1. e4 c5 2. Nf3 d6", "e4 c5 Nf3 d6"},
		{"1.e4 c5 2.Nf3 2...d6", "e4 c5 Nf3 d6"},
		{"e4 c5 * Nf3", "e4 c5"},
		{"e4 c5 *", "e4 c5"},
		{"1. 2.", ""},
		{"  e4\tc5  ", "e4 c5"},
	}
	for _, c := range cases {
		if got := strings.Join(positionalLineMoves(c.in), " "); got != c.want {
			t.Errorf("positionalLineMoves(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAddLineDepthAndMatch(t *testing.T) {
	pc := lineCriteria(t, najdorfLine)
	if pc.LineDepth() != 19 || pc.effectiveLimit() != 19 {
		t.Fatalf("depth %d limit %d, want 19", pc.LineDepth(), pc.effectiveLimit())
	}
	if pc.Empty() {
		t.Fatal("Empty after AddLine")
	}

	g, r := replayed(t, najdorfGame)
	ok, err := Evaluate(g, r, &Options{Positions: pc, MarkMatches: "MATCH"})
	if err != nil || !ok {
		t.Fatalf("matched %v err %v", ok, err)
	}
	if got := commentsAt(g); len(got) != 1 || got[0] != 10 || g.Moves[10].Comments[0] != "MATCH" {
		t.Errorf("mark at %v: %v", got, g.Moves[10].Comments)
	}

	// A line of only move numbers still raises the depth to 8 but adds
	// nothing (compose_positional_variation).
	pc2 := &PositionCriteria{}
	if err := pc2.AddLine("1. 2."); err != nil || !pc2.Empty() || pc2.LineDepth() != 8 {
		t.Errorf("number-only line: err %v empty %v depth %d", err, pc2.Empty(), pc2.LineDepth())
	}
}

func TestAddLineTranspositionBeyondDepth(t *testing.T) {
	// Sanity: the transposed game really reaches the Be3 placement at ply 25.
	_, straight := replayed(t, najdorfGame)
	gT, rT := replayed(t, najdorfTransposed)
	if straight.Positions[11].Placement != rT.Positions[25].Placement {
		t.Fatalf("transposition does not reach the target placement")
	}

	pc := lineCriteria(t, najdorfLine)
	ok, _ := Evaluate(gT, rT, &Options{Positions: pc, MarkMatches: "X"})
	if ok {
		t.Error("matched beyond the implied depth of 19")
	}
	if n := commentsAt(gT); len(n) != 0 {
		t.Errorf("marks after an unmatched walk: %v", n)
	}

	pc.MatchPlyLimit = 30
	gT, rT = replayed(t, najdorfTransposed)
	ok, _ = Evaluate(gT, rT, &Options{Positions: pc, MarkMatches: "X"})
	if !ok {
		t.Fatal("not matched with MatchPlyLimit 30")
	}
	if got := commentsAt(gT); len(got) != 1 || got[0] != 24 {
		t.Errorf("mark at %v, want [24]", got)
	}
}

func TestAddLineErrors(t *testing.T) {
	pc := &PositionCriteria{}
	if err := pc.AddLine("e4 e5 Qh9"); err == nil {
		t.Error("illegal move accepted")
	}
	if err := pc.AddLine("e4 e5 Nf3 Nc6 Bb5 a6 Ba4 Nf6 O-O Be7 Re1 b5 Bb3 d6 c3 O-O h3 Na5 Bc2 c5 d4 Qc7"); err != nil {
		t.Errorf("Marshall/Chigorin line rejected: %v", err)
	}
	if pc.LineDepth() != 30 {
		t.Errorf("depth %d, want 30", pc.LineDepth())
	}
}

func TestEffectiveLimit(t *testing.T) {
	pc := &PositionCriteria{}
	if pc.effectiveLimit() != DefaultPositionalDepth {
		t.Errorf("default limit %d", pc.effectiveLimit())
	}
	pc.AddLine("e4 e5")
	if pc.effectiveLimit() != 10 {
		t.Errorf("line limit %d, want 10", pc.effectiveLimit())
	}
	pc.MatchPlyLimit = 40
	if pc.effectiveLimit() != 40 {
		t.Errorf("explicit limit %d, want 40", pc.effectiveLimit())
	}
}

func TestAddHash(t *testing.T) {
	pc := &PositionCriteria{}
	for _, bad := range []string{"", "xyz", "db55d4fcaadc775e0", "-1", "0x12", "12 "} {
		if err := pc.AddHash(bad); !errors.Is(err, ErrBadHash) {
			t.Errorf("AddHash(%q) = %v, want ErrBadHash", bad, err)
		}
	}
	if err := pc.AddHash("DB55D4FCAADC775E"); err != nil {
		t.Fatal(err)
	}
	if err := pc.AddHash("f"); err != nil {
		t.Fatal(err)
	}
	if _, ok := pc.hashes[0xdb55d4fcaadc775e]; !ok {
		t.Error("upper-case hash not stored")
	}
	if _, ok := pc.hashes[0xf]; !ok {
		t.Error("short hash not stored")
	}
}

func TestHashMatchObeysPlyLimit(t *testing.T) {
	// test-matchplylimit: db55d4fcaadc775e is the position after 5. Nc3
	// (ply 9) in the Kopavogur game.
	const game = "1. e4 c5 2. Nf3 d6 3. d4 cxd4 4. Nxd4 Nf6 5. Nc3 a6 6. Be3 e6 7. f3 Nbd7 *\n"
	for _, c := range []struct {
		limit int
		want  bool
	}{{8, false}, {9, true}, {10, true}, {150, true}} {
		pc := &PositionCriteria{MatchPlyLimit: c.limit}
		if err := pc.AddHash("db55d4fcaadc775e"); err != nil {
			t.Fatal(err)
		}
		g, r := replayed(t, game)
		ok, err := Evaluate(g, r, &Options{Positions: pc, MarkMatches: "match"})
		if err != nil || ok != c.want {
			t.Errorf("limit %d: matched %v err %v, want %v", c.limit, ok, err, c.want)
		}
		if c.want {
			if got := commentsAt(g); len(got) != 1 || got[0] != 8 {
				t.Errorf("limit %d: mark at %v, want [8]", c.limit, got)
			}
		}
	}
}

func TestAddFENIgnoresSideToMove(t *testing.T) {
	// The placement after 1. e4, given with Black to move.
	pc := &PositionCriteria{}
	if err := pc.AddFEN("rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1"); err != nil {
		t.Fatal(err)
	}
	if err := pc.AddFEN("not a fen"); err == nil {
		t.Error("bad FEN accepted")
	}
	// The same placement with White to move, as a game's start position.
	whiteToMove := "[FEN \"rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR w KQkq - 0 1\"]\n\n1. Nf3 Nc6 *\n"
	mustMatch(t, whiteToMove, &Options{Positions: pc}, true)
	mustMatch(t, "1. e4 e5 *\n", &Options{Positions: pc}, true)
	mustMatch(t, "1. d4 e5 *\n", &Options{Positions: pc}, false)

	pc.WhoseMove = BlackToMove
	mustMatch(t, whiteToMove, &Options{Positions: pc}, false)
	mustMatch(t, "1. e4 e5 *\n", &Options{Positions: pc}, true)
	pc.WhoseMove = WhiteToMove
	mustMatch(t, whiteToMove, &Options{Positions: pc}, true)
	mustMatch(t, "1. e4 e5 *\n", &Options{Positions: pc}, false)
}

func TestWhoseMoveFallsThroughToPatterns(t *testing.T) {
	// Found by key but the wrong side to move: position_matches still
	// tries the patterns, which match a later position with the right side.
	pc := &PositionCriteria{WhoseMove: BlackToMove}
	if err := pc.AddFEN("rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR w KQkq - 0 1"); err != nil {
		t.Fatal(err)
	}
	src := "[FEN \"rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR w KQkq - 0 1\"]\n\n1. Nf3 Nc6 *\n"
	mustMatch(t, src, &Options{Positions: pc}, false)

	if err := pc.AddPattern("*/*/*/*/*/*/*/*", false, "ANY"); err != nil {
		t.Fatal(err)
	}
	g, r := replayed(t, src)
	ok, err := Evaluate(g, r, &Options{Positions: pc, MarkMatches: "M", AddLabelTag: true})
	if err != nil || !ok {
		t.Fatalf("matched %v err %v", ok, err)
	}
	// Start position: key hit, wrong side; pattern hit, wrong side → no.
	// After 1. Nf3 (Black to move): pattern hit → marked and labelled.
	if len(g.PrefixComments) != 0 || len(g.Moves[0].Comments) != 1 {
		t.Errorf("prefix %v move0 %v", g.PrefixComments, g.Moves[0].Comments)
	}
	if v, _ := g.Tag("MatchLabel"); v != "ANY" {
		t.Errorf("MatchLabel = %q", v)
	}
}

func TestStartPositionMatchPrependsPrefixComment(t *testing.T) {
	pc := &PositionCriteria{}
	if err := pc.AddFEN(initialFEN); err != nil {
		t.Fatal(err)
	}
	g, r := replayed(t, "{intro} 1. e4 e5 *\n")
	ok, err := Evaluate(g, r, &Options{Positions: pc, MarkMatches: "MARK"})
	if err != nil || !ok {
		t.Fatalf("matched %v err %v", ok, err)
	}
	if len(g.PrefixComments) != 2 || g.PrefixComments[0] != "MARK" || g.PrefixComments[1] != "intro" {
		t.Errorf("prefix comments %v", g.PrefixComments)
	}
	if n := commentsAt(g); len(n) != 0 {
		t.Errorf("move marks after a start match: %v", n)
	}

	// "FEN" as the mark inserts the position's FEN.
	g, r = replayed(t, "1. e4 e5 *\n")
	Evaluate(g, r, &Options{Positions: pc, MarkMatches: "FEN"})
	if len(g.PrefixComments) != 1 || g.PrefixComments[0] != initialFEN {
		t.Errorf("FEN mark: %v", g.PrefixComments)
	}

	// StartPly 2 excludes the start position (ply 1); the game continues
	// to be searched but never returns to the start.
	mustMatch(t, "1. e4 e5 *\n", &Options{Positions: pc, StartPly: 2}, false)
}

func TestStartPlyGatesRepetitionMarks(t *testing.T) {
	const shuffle = "1. Nf3 Nf6 2. Ng1 Ng8 3. Nf3 Nf6 4. Ng1 Ng8 5. Nf3 Nf6 6. Ng1 Ng8 *\n"
	g, r := replayed(t, shuffle)
	ok, _ := Evaluate(g, r, &Options{Repetition: 3, MarkMatches: "R"})
	if !ok {
		t.Fatal("threefold not found")
	}
	if got := commentsAt(g); len(got) != 5 || got[0] != 7 || got[4] != 11 {
		t.Errorf("StartPly 1 marks at %v, want [7 8 9 10 11]", got)
	}

	g, r = replayed(t, shuffle)
	ok, _ = Evaluate(g, r, &Options{Repetition: 3, MarkMatches: "R", StartPly: 8})
	if !ok {
		t.Fatal("threefold not found from ply 8")
	}
	if got := commentsAt(g); len(got) != 1 || got[0] != 11 {
		t.Errorf("StartPly 8 marks at %v, want [11]", got)
	}

	// From ply 9 only two occurrences of the start position are counted.
	mustMatch(t, shuffle, &Options{Repetition: 3, StartPly: 9}, false)
}

func TestStartPlyGatesNMoveAndUnderpromotion(t *testing.T) {
	src := "[FEN \"4k3/8/8/8/8/8/8/4K2R w K - 98 60\"]\n\n60. Kd1 Kd8 61. Ke1 Ke8 *\n"
	// First move is ply 119; the clock reaches 100 after it.
	mustMatch(t, src, &Options{NMoveRule: 50, StartPly: 119}, true)
	mustMatch(t, src, &Options{NMoveRule: 50, StartPly: 123}, false)

	under := "[FEN \"8/P6k/8/8/8/8/8/K7 w - - 0 1\"]\n\n1. a8=R Kg7 *\n"
	mustMatch(t, under, &Options{Underpromotion: true, StartPly: 1}, true)
	mustMatch(t, under, &Options{Underpromotion: true, StartPly: 2}, false)
}

func TestMatchInsideVariation(t *testing.T) {
	pc := &PositionCriteria{}
	if err := pc.AddPattern("*/*/*/3p4/3P4/*/*/*", false, "QP"); err != nil {
		t.Fatal(err)
	}
	src := "1. e4 (1. d4 d5 2. c4) e5 2. Nf3 *\n"
	g, r := replayed(t, src)
	ok, err := Evaluate(g, r, &Options{Positions: pc, MarkMatches: "V", AddLabelTag: true})
	if err != nil || !ok {
		t.Fatalf("matched %v err %v", ok, err)
	}
	rav := g.Moves[0].Variations[0].Moves
	if len(rav[1].Comments) != 1 || rav[1].Comments[0] != "V" {
		t.Errorf("RAV marks: %v / %v / %v", rav[0].Comments, rav[1].Comments, rav[2].Comments)
	}
	if n := commentsAt(g); len(n) != 0 {
		t.Errorf("main-line marks after a variation match: %v", n)
	}
	if _, has := g.Tag("MatchLabel"); has {
		t.Error("a variation match must not contribute a label")
	}

	// -V: variations are not searched.
	mustMatch(t, src, &Options{Positions: pc, NoVariations: true}, false)

	// A variation match suppresses the main-line mark at the same move.
	pc2 := &PositionCriteria{}
	pc2.AddLine("d4 d5")
	pc2.AddLine("e4")
	g, r = replayed(t, src)
	Evaluate(g, r, &Options{Positions: pc2, MarkMatches: "V"})
	if len(g.Moves[0].Comments) != 0 || len(g.Moves[0].Variations[0].Moves[1].Comments) != 1 {
		t.Errorf("main %v rav %v", g.Moves[0].Comments, g.Moves[0].Variations[0].Moves[1].Comments)
	}

	// Nested variations are searched too, with the default depth: the
	// main-line limit of 2 lets the walk reach the move the variation
	// hangs off, and the target at ply 4 is found inside it.
	nested := "1. e4 e5 (1... c5 2. Nf3 (2. c3 d5)) 2. Nf3 *\n"
	pc3 := &PositionCriteria{MatchPlyLimit: 2}
	pc3.AddLine("e4 c5 c3 d5")
	mustMatch(t, nested, &Options{Positions: pc3}, true)
	// With limit 1 the walk stops before the move carrying the variation.
	pc3.MatchPlyLimit = 1
	mustMatch(t, nested, &Options{Positions: pc3}, false)
}

func TestAddLabelTag(t *testing.T) {
	pc := &PositionCriteria{}
	pc.AddPattern("*/*/*/*/4P3/*/*/*", false, "KP")
	g, r := replayed(t, "1. e4 e5 *\n")
	Evaluate(g, r, &Options{Positions: pc, AddLabelTag: true})
	if v, _ := g.Tag("MatchLabel"); v != "KP" {
		t.Errorf("MatchLabel = %q", v)
	}

	g, r = replayed(t, "1. e4 e5 *\n")
	Evaluate(g, r, &Options{Positions: pc})
	if _, has := g.Tag("MatchLabel"); has {
		t.Error("tag added without AddLabelTag")
	}

	unlabelled := &PositionCriteria{}
	unlabelled.AddPattern("*/*/*/*/4P3/*/*/*", false, "")
	unlabelled.AddLine("e4")
	g, r = replayed(t, "1. e4 e5 *\n")
	ok, _ := Evaluate(g, r, &Options{Positions: unlabelled, AddLabelTag: true})
	if _, has := g.Tag("MatchLabel"); !ok || has {
		t.Errorf("matched %v, tag present %v", ok, has)
	}

	// The tag is added even when a later criterion rejects the game, as
	// play_moves sets it before the N-move/underpromotion checks.
	g, r = replayed(t, "1. e4 e5 *\n")
	ok, _ = Evaluate(g, r, &Options{Positions: pc, AddLabelTag: true, Checkmate: true})
	if v, _ := g.Tag("MatchLabel"); ok || v != "KP" {
		t.Errorf("matched %v MatchLabel %q", ok, v)
	}
}

func TestInverseLabelSuffix(t *testing.T) {
	pc := &PositionCriteria{}
	if err := pc.AddPattern("*/*/*/*/4P3/*/*/*", true, "KP"); err != nil {
		t.Fatal(err)
	}
	g, r := replayed(t, "1. d4 e5 *\n")
	ok, _ := Evaluate(g, r, &Options{Positions: pc, AddLabelTag: true})
	if v, _ := g.Tag("MatchLabel"); !ok || v != "KPI" {
		t.Errorf("matched %v MatchLabel %q", ok, v)
	}
}

func TestLimitStopSkipsResultCheck(t *testing.T) {
	pc := &PositionCriteria{MatchPlyLimit: 2}
	pc.AddFEN("8/8/8/8/8/8/8/K6k w - - 0 1") // never reached
	src := "[Result \"1-0\"]\n\n1. e4 e5 2. Nf3 Nc6 0-1\n"
	ok, err := eval(t, src, &Options{Positions: pc, NoBadResults: true})
	if ok || err != nil {
		t.Errorf("stopped walk: matched %v err %v", ok, err)
	}
	pc.MatchPlyLimit = 0
	_, err = eval(t, src, &Options{Positions: pc, NoBadResults: true})
	if !errors.Is(err, ErrBadResult) {
		t.Errorf("full walk: err %v, want ErrBadResult", err)
	}

	// Once matched the walk runs to the end regardless of the limit, so
	// the result check and later marks happen.
	pc2 := &PositionCriteria{MatchPlyLimit: 1}
	pc2.AddLine("e4")
	g, r := replayed(t, "[Result \"1-0\"]\n\n1. e4 e5 2. Nf3 Nc6 0-1\n")
	_, err = Evaluate(g, r, &Options{Positions: pc2, NoBadResults: true})
	if !errors.Is(err, ErrBadResult) {
		t.Errorf("matched walk: err %v, want ErrBadResult", err)
	}
}

func TestLimitStopConfinesLaterMarks(t *testing.T) {
	// Repetition marks are only inserted for plies the walk reached.
	pc := &PositionCriteria{MatchPlyLimit: 3}
	pc.AddFEN("8/8/8/8/8/8/8/K6k w - - 0 1")
	g, r := replayed(t, "1. Nf3 Nf6 2. Ng1 Ng8 3. Nf3 Nf6 4. Ng1 Ng8 5. Nf3 Nf6 6. Ng1 Ng8 *\n")
	ok, _ := Evaluate(g, r, &Options{Positions: pc, Repetition: 3, MarkMatches: "R"})
	if ok || len(commentsAt(g)) != 0 {
		t.Errorf("matched %v marks %v", ok, commentsAt(g))
	}
}

func TestPositionMarkPrecedesRepetitionMark(t *testing.T) {
	pc := &PositionCriteria{}
	pc.AddFEN(initialFEN)
	// Start position matches at ply 1 and is excluded by StartPly 4; the
	// first return to it is after 2... Ng8 (index 3), where the position
	// mark and the twofold mark coincide.
	g, r := replayed(t, "1. Nf3 Nf6 2. Ng1 Ng8 3. Nf3 *\n")
	ok, _ := Evaluate(g, r, &Options{Positions: pc, Repetition: 2, MarkMatches: "M", StartPly: 4})
	if !ok {
		t.Fatal("not matched")
	}
	if len(g.PrefixComments) != 0 {
		t.Errorf("prefix %v", g.PrefixComments)
	}
	if c := g.Moves[3].Comments; len(c) != 2 {
		t.Errorf("move 3 comments %v, want position mark then repetition mark", c)
	}
}

func TestReadPositionLines(t *testing.T) {
	in := "% comment\n\ne4 c5 Nf3\r\n   \nd4 d5 *\n"
	lines, err := ReadPositionLines(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0] != "e4 c5 Nf3" || lines[1] != "d4 d5 *" {
		t.Errorf("lines %q", lines)
	}
}

func TestMatchPositionsNilAndPartial(t *testing.T) {
	pc := &PositionCriteria{}
	pc.AddLine("e4 e5 Nf3")
	g, _ := replayed(t, "1. e4 e5 *\n")
	if m, _, stop := matchPositions(g, nil, pc, 1, "", false); m || stop != 0 {
		t.Errorf("nil result: %v %d", m, stop)
	}
	// Broken game kept with --keep-broken: the walk covers the replayed prefix.
	g.Moves = append(g.Moves, &pgn.Move{SAN: "Nf3"}, &pgn.Move{SAN: "Qh9"})
	r, err := replay.Game(g, replay.Options{})
	if err == nil {
		t.Fatal("expected replay failure")
	}
	m, _, stop := matchPositions(g, r, pc, 1, "P", false)
	if !m || stop != 3 || len(g.Moves[2].Comments) != 1 {
		t.Errorf("partial: matched %v stop %d marks %v", m, stop, commentsAt(g))
	}
	if r.Positions[3].SideToMove != chess.Black {
		t.Error("sanity: after Nf3 Black is to move")
	}
}

func TestPlyNumber(t *testing.T) {
	_, r := replayed(t, "1. e4 e5 2. Nf3 *\n")
	for i, want := range []int{1, 2, 3, 4} {
		if got := plyNumber(r.Positions[i]); got != want {
			t.Errorf("plyNumber(pos %d) = %d, want %d", i, got, want)
		}
	}
	_, r = replayed(t, "[FEN \"4k3/8/8/8/8/8/8/4K2R b K - 0 60\"]\n\n60... Kd8 61. Ke2 *\n")
	if got := plyNumber(r.Positions[0]); got != 120 {
		t.Errorf("black-to-move start ply %d, want 120", got)
	}
}
