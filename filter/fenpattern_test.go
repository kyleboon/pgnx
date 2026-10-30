package filter

import (
	"errors"
	"testing"

	"github.com/kyleboon/pgnx/chess"
)

const startFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"

func board(t *testing.T, fen string) *chess.Board {
	t.Helper()
	b, err := chess.FromFEN(fen)
	if err != nil {
		t.Fatalf("FromFEN(%q): %v", fen, err)
	}
	return b
}

func TestPatternSetAddGrammar(t *testing.T) {
	cases := []struct {
		pattern string
		wantErr bool
	}{
		{"*/*/*/*/*/*/*/*", false},
		{"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR", false},
		{"*/*/*/*/*/*/*", true},     // 7 ranks
		{"*/*/*/*/*/*/*/*/*", true}, // 9 ranks
		{"*//*/*/*/*/*/*", true},    // empty rank
		{"*/*/*/*/*/*/*/", true},    // empty final rank
		{"[[NB]*/*/*/*/*/*/*/*", true},
		{"]*/*/*/*/*/*/*/*", true},
		{"[NB*/*/*/*/*/*/*/*", true}, // unterminated closure
		{"^*/*/*/*/*/*/*/*", true},
		{"[^P_]*/*/*/*/*/*/*/*", false},
		{"", true},
		{"*/*/*/*/*/*/*/* w KQkq - 0 1", false}, // trailing fields ignored
		{"*/*/*/*/*/*/*/*:kq k", false},
		{"*/*/*/*/*/*/*/*:xyz", true}, // bad constraint
	}
	for _, c := range cases {
		var s PatternSet
		err := s.Add(c.pattern, false, "")
		if (err != nil) != c.wantErr {
			t.Errorf("Add(%q) error = %v, wantErr %v", c.pattern, err, c.wantErr)
		}
		if c.wantErr && !s.Empty() {
			t.Errorf("Add(%q) failed but set is not empty", c.pattern)
		}
	}
}

func TestPatternSetAddConstraintParsed(t *testing.T) {
	var s PatternSet
	if err := s.Add("*/*/*/*/*/*/*/*:kq k", false, "L"); err != nil {
		t.Fatal(err)
	}
	n := s.root
	for n.next != nil {
		n = n.next
	}
	if n.constraint == nil {
		t.Fatal("constraint not stored on the final rank")
	}
	if n.constraint.num[0][chess.Queen] != 1 || n.constraint.num[1][chess.Queen] != 0 || n.constraint.bothColours {
		t.Errorf("constraint = %+v", n.constraint)
	}
	if n.label != "L" {
		t.Errorf("label = %q", n.label)
	}

	// Inverse patterns get a colour-symmetric constraint.
	var inv PatternSet
	if err := inv.Add("*/*/*/*/*/*/*/K*:kq k", true, ""); err != nil {
		t.Fatal(err)
	}
	for n := inv.root; n != nil; n = n.next {
		if n.next == nil && !n.constraint.bothColours {
			t.Error("inverse constraint should match either colour")
		}
	}
}

// Knight on e4 supported by a pawn on d3 (White), from 1.e4 e5 2.Nf3 Nc6
// 3.d3 Nf6 4.Ng5 ... hypothetical; the FEN is hand-built.
const knightE4 = "r1bqkb1r/pppp1ppp/2n2n2/4p3/4N3/3P4/PPP2PPP/R1BQKB1R b KQkq - 0 5"

// Same but the pawn is on c3 rather than d3: not supporting.
const knightE4NoSupport = "r1bqkb1r/pppp1ppp/2n2n2/4p3/4N3/2P5/PP3PPP/R1BQKB1R b KQkq - 0 5"

func TestPatternSetMatch(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		fen     string
		want    bool
	}{
		{"all wildcards", "*/*/*/*/*/*/*/*", startFEN, true},
		{"literal start", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR", startFEN, true},
		{"literal with underscores", "rnbqkbnr/pppppppp/________/8/8/8/PPPPPPPP/RNBQKBNR", startFEN, true},
		{"knight e4 supported", "*/*/*/*/????N???/???P????/*/*", knightE4, true},
		{"knight e4 unsupported", "*/*/*/*/????N???/???P????/*/*", knightE4NoSupport, false},
		{"digit exact ok", "*/*/*/*/4N3/*/*/*", knightE4, true},
		{"digit exact too many", "*/*/*/*/5N2/*/*/*", knightE4, false},
		{"8 vs rank with 7 empties", "*/*/*/*/8/*/*/*", knightE4, false},
		{"8 matches empty rank", "*/*/*/*/8/*/*/*", "k7/8/8/8/8/8/8/K7 w - - 0 1", true},
		{"any-square", "*/*/*/*/????????/*/*/*", knightE4, true},
		{"non-empty", "*/*/*/*/????!???/*/*/*", knightE4, true},
		{"non-empty fails on empty", "*/*/*/*/???!????/*/*/*", knightE4, false},
		{"any white", "*/*/*/*/????A???/*/*/*", knightE4, true},
		{"any white fails on black", "*/*/*/????a???/*/*/*/*", knightE4, true}, // e5 black pawn
		{"any black fails on white", "*/*/*/*/????a???/*/*/*", knightE4, false},
		{"not a pawn", "*/*/*/*/????m???/*/*/*", knightE4, true},
		{"not a pawn fails on pawn", "*/*/*/????m???/*/*/*/*", knightE4, false},
		{"not a pawn accepts empty", "*/*/*/*/m*/*/*/*", knightE4, true},
		{"closure", "*/*/*/*/????[NB]???/*/*/*", knightE4, true},
		{"closure miss", "*/*/*/*/????[RB]???/*/*/*", knightE4, false},
		{"negated closure", "*/*/*/*/????[^P_]???/*/*/*", knightE4, true},
		{"negated closure miss", "*/*/*/*/????[^N_]???/*/*/*", knightE4, false},
		{"closure with underscore", "*/*/*/*/???[P_]N???/*/*/*", knightE4, true},
		{"star at both ends", "*/*/*/*/*N*/*/*/*", knightE4, true},
		{"star in middle", "*/*/*/*/4N*/*/*/*", knightE4, true},
		{"star leading", "*/*/*/*/*N3/*/*/*", knightE4, true},
		{"star zero width", "*/*/*/*/*4N3*/*/*/*", knightE4, true},
		{"pattern must consume rank", "*/*/*/*/4N/*/*/*", knightE4, false},
		{"pattern too long", "*/*/*/*/4N3??/*/*/*", knightE4, false},
		{"rank 8 first", "r1bqkb1r/*/*/*/*/*/*/R1BQKB1R", knightE4, true},
		{"rank order matters", "R1BQKB1R/*/*/*/*/*/*/r1bqkb1r", knightE4, false},
		{"constraint satisfied", "*/*/*/*/*/*/*/*:KQR2B2N2P8 kqr2b2n2p8", startFEN, true},
		{"constraint rejects rank match", "*/*/*/*/*/*/*/*:KQ k", startFEN, false},
		{"constraint white-first", "*/*/*/*/*/*/*/*:kq k", "8/8/8/8/8/8/8/KQ5k w - - 0 1", true},
		{"constraint white-first not black", "*/*/*/*/*/*/*/*:kq k", "8/8/8/8/8/8/8/kq5K w - - 0 1", false},
	}
	for _, c := range cases {
		var s PatternSet
		if err := s.Add(c.pattern, false, "L"); err != nil {
			t.Errorf("%s: Add: %v", c.name, err)
			continue
		}
		label, ok := s.Match(board(t, c.fen))
		if ok != c.want {
			t.Errorf("%s: Match = %v, want %v", c.name, ok, c.want)
		}
		if ok && label != "L" {
			t.Errorf("%s: label = %q", c.name, label)
		}
	}
}

func TestPatternSetEmptyNoMatch(t *testing.T) {
	var s PatternSet
	if !s.Empty() {
		t.Error("zero PatternSet should be empty")
	}
	if _, ok := s.Match(board(t, startFEN)); ok {
		t.Error("empty set matched")
	}
}

func TestPatternSetInverse(t *testing.T) {
	var s PatternSet
	if err := s.Add("*/*/*/*/???Np???/????P???/*/*", true, "X"); err != nil {
		t.Fatal(err)
	}
	// White knight on d4 supported by e3 pawn, black pawn on e4.
	white := "k7/8/8/8/3Np3/4P3/8/K7 w - - 0 1"
	if label, ok := s.Match(board(t, white)); !ok || label != "X" {
		t.Errorf("white form: (%q, %v)", label, ok)
	}
	// Mirror: black knight on d5 supported by e6 pawn, white pawn on e5.
	black := "k7/8/4p3/3nP3/8/8/8/K7 w - - 0 1"
	if label, ok := s.Match(board(t, black)); !ok || label != "XI" {
		t.Errorf("black form: (%q, %v)", label, ok)
	}

	// An unlabelled pattern's inverse is labelled "I", as --fenpatterni
	// does in C (add_fen_pattern receives "" and appends "I").
	var e PatternSet
	if err := e.Add("*/*/*/*/???Np???/????P???/*/*", true, ""); err != nil {
		t.Fatal(err)
	}
	if label, ok := e.Match(board(t, white)); !ok || label != "" {
		t.Errorf("unlabelled original: (%q, %v)", label, ok)
	}
	if label, ok := e.Match(board(t, black)); !ok || label != "I" {
		t.Errorf("unlabelled inverse: (%q, %v)", label, ok)
	}

	// An unlabelled FENPatternI criteria line leaves the inverse
	// unlabelled too (taglines.c passes a NULL label; see AddCriterion).
	for _, line := range []string{
		`FENPatternI "*/*/*/*/???Np???/????P???/*/*"`,
		`FENPatternI "*/*/*/*/???Np???/????P???/*/*"   `,
	} {
		c, err := ParseCriterion(line)
		if err != nil {
			t.Fatal(err)
		}
		var pc PositionCriteria
		if err := pc.AddCriterion(c); err != nil {
			t.Fatal(err)
		}
		if label, ok := pc.Patterns.Match(board(t, black)); !ok || label != "" {
			t.Errorf("%q inverse: (%q, %v)", line, label, ok)
		}
	}
	labelled, err := ParseCriterion(`FENPatternI "*/*/*/*/???Np???/????P???/*/*" Nd4th`)
	if err != nil {
		t.Fatal(err)
	}
	var pc PositionCriteria
	if err := pc.AddCriterion(labelled); err != nil {
		t.Fatal(err)
	}
	if label, ok := pc.Patterns.Match(board(t, black)); !ok || label != "Nd4thI" {
		t.Errorf("labelled line inverse: (%q, %v)", label, ok)
	}
	if err := pc.AddCriterion(Criterion{Tag: "White", Value: "x"}); !errors.Is(err, ErrNotPositionalCriterion) {
		t.Errorf("tag criterion: err = %v", err)
	}

	// Inverse of a constrained pattern keeps the constraint (C drops the
	// whole inverse pattern here; see CONFORMANCE.md).
	var c PatternSet
	if err := c.Add("*/*/*/*/*/*/*/K*:kq k", true, "Q"); err != nil {
		t.Fatal(err)
	}
	if label, ok := c.Match(board(t, "8/8/8/8/8/8/8/KQ5k w - - 0 1")); !ok || label != "Q" {
		t.Errorf("original form, white has the queen: (%q, %v)", label, ok)
	}
	if label, ok := c.Match(board(t, "kq5K/8/8/8/8/8/8/8 w - - 0 1")); !ok || label != "QI" {
		t.Errorf("inverse form, black has the queen: (%q, %v)", label, ok)
	}
	if _, ok := c.Match(board(t, "kq4RK/8/8/8/8/8/8/8 w - - 0 1")); ok {
		t.Error("inverse constraint should reject the extra rook")
	}
	if _, ok := c.Match(board(t, "8/8/8/8/8/8/8/kq5K w - - 0 1")); ok {
		t.Error("rank pattern (white king on a1) should not match the mirrored board")
	}
}

func TestReverseFENPattern(t *testing.T) {
	cases := []struct{ in, want string }{
		{"*/*/*/*/???Np???/????P???/*/*", "*/*/????p???/???nP???/*/*/*/*"},
		{"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR"},
		{"K7/*/*/*/*/*/*/[Aa]7", "[aA]7/*/*/*/*/*/*/k7"},
		{"m7/*/*/*/*/*/*/8", "8/*/*/*/*/*/*/m7"}, // m is colour-neutral
	}
	for _, c := range cases {
		if got := reverseFENPattern(c.in); got != c.want {
			t.Errorf("reverseFENPattern(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPatternSetTrieOrder(t *testing.T) {
	// P1 and P3 share rank 8 and differ at rank 7; P2 differs at rank 8.
	// C walks P1, then P1's alternatives at rank 7 (P3), then P2.
	var s PatternSet
	must := func(p, label string) {
		t.Helper()
		if err := s.Add(p, false, label); err != nil {
			t.Fatal(err)
		}
	}
	must("k7/8/*/*/*/*/*/*", "P1")
	must("*/*/*/*/*/*/*/*", "P2")
	must("k7/*/*/*/*/*/*/*", "P3")

	// Matches P2 and P3 (rank 7 non-empty), not P1.
	if label, ok := s.Match(board(t, "k7/p7/8/8/8/8/8/K7 w - - 0 1")); !ok || label != "P3" {
		t.Errorf("got (%q, %v), want P3", label, ok)
	}
	// Matches all three: P1 is first.
	if label, ok := s.Match(board(t, "k7/8/8/8/8/8/8/K7 w - - 0 1")); !ok || label != "P1" {
		t.Errorf("got (%q, %v), want P1", label, ok)
	}
	// Only P2.
	if label, ok := s.Match(board(t, "7k/8/8/8/8/8/8/K7 w - - 0 1")); !ok || label != "P2" {
		t.Errorf("got (%q, %v), want P2", label, ok)
	}
}

func TestPatternSetDuplicate(t *testing.T) {
	var s PatternSet
	if err := s.Add("*/*/*/*/*/*/*/*", false, "first"); err != nil {
		t.Fatal(err)
	}
	err := s.Add("*/*/*/*/*/*/*/*", false, "second")
	if !errors.Is(err, ErrDuplicatePattern) {
		t.Fatalf("duplicate Add error = %v", err)
	}
	if label, ok := s.Match(board(t, startFEN)); !ok || label != "first" {
		t.Errorf("after duplicate: (%q, %v)", label, ok)
	}
	// A duplicate reported through the inverse path too.
	var sym PatternSet
	if err := sym.Add("*/*/*/*/*/*/*/*", true, ""); !errors.Is(err, ErrDuplicatePattern) {
		t.Errorf("self-symmetric inverse should report a duplicate, got %v", err)
	}
	if sym.Empty() {
		t.Error("original pattern must still be stored")
	}
}

func TestMaterialMatchBoard(t *testing.T) {
	cases := []struct {
		desc    string
		colours Colours
		fen     string
		want    bool
	}{
		{"kq k", WhiteFirst, "8/8/8/8/8/8/8/KQ5k w - - 0 1", true},
		{"kq k", WhiteFirst, "8/8/8/8/8/8/8/kq5K w - - 0 1", false},
		{"kq k", EitherColour, "8/8/8/8/8/8/8/kq5K w - - 0 1", true},
		{"kq k", WhiteFirst, "8/8/8/8/8/8/8/KQ4Rk w - - 0 1", false},
		{"kr* kp3", WhiteFirst, "8/8/8/8/8/ppp5/8/KRR4k w - - 0 1", true},
		{"kr* kp3", WhiteFirst, "8/8/8/8/8/pppp4/8/KRR4k w - - 0 1", false},
		// Depth is irrelevant for a single-position check.
		{"5 kq k", WhiteFirst, "8/8/8/8/8/8/8/KQ5k w - - 0 1", true},
	}
	for _, c := range cases {
		p, err := ParseMaterial(c.desc, c.colours)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.MatchBoard(board(t, c.fen)); got != c.want {
			t.Errorf("MatchBoard(%q, %v, %s) = %v, want %v", c.desc, c.colours, c.fen, got, c.want)
		}
	}
}
