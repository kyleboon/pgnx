package filter

import (
	"strings"
	"testing"

	"github.com/kyleboon/pgnx/chess"
)

func TestParseMaterialGrammar(t *testing.T) {
	p, err := ParseMaterial("q*r*p*b2n2< q=r=p=b2<n2", EitherColour)
	if err != nil {
		t.Fatal(err)
	}
	if p.moveDepth != 2 || !p.bothColours {
		t.Errorf("depth/colours: %+v", p)
	}
	type want struct {
		n   int
		occ occurs
	}
	set0 := map[chess.PieceType]want{
		chess.Queen: {0, numOrMore}, chess.Rook: {0, numOrMore}, chess.Pawn: {0, numOrMore},
		chess.Bishop: {2, exactly}, chess.Knight: {2, lessThanOpp}, chess.King: {1, exactly},
	}
	for piece, w := range set0 {
		if p.num[0][piece] != w.n || p.occ[0][piece] != w.occ {
			t.Errorf("set0 %v = (%d,%d), want (%d,%d)", piece, p.num[0][piece], p.occ[0][piece], w.n, w.occ)
		}
	}
	set1 := map[chess.PieceType]want{
		chess.Queen: {1, sameAsOpponent}, chess.Rook: {1, sameAsOpponent}, chess.Pawn: {1, sameAsOpponent},
		chess.Bishop: {2, lessThanOpp}, chess.Knight: {2, exactly}, chess.King: {1, exactly},
	}
	for piece, w := range set1 {
		if p.num[1][piece] != w.n || p.occ[1][piece] != w.occ {
			t.Errorf("set1 %v = (%d,%d), want (%d,%d)", piece, p.num[1][piece], p.occ[1][piece], w.n, w.occ)
		}
	}

	p, err = ParseMaterial("3 kr k", WhiteFirst)
	if err != nil {
		t.Fatal(err)
	}
	if p.moveDepth != 3 || p.bothColours || p.num[0][chess.Rook] != 1 || p.num[1][chess.Rook] != 0 || p.num[1][chess.King] != 1 {
		t.Errorf("3 kr k: %+v", p)
	}

	p, err = ParseMaterial("KQ", EitherColour)
	if err != nil {
		t.Fatal(err)
	}
	if p.num[0][chess.Queen] != 1 || p.num[1][chess.Queen] != 0 || p.num[1][chess.King] != 1 {
		t.Errorf("KQ lone king opponent: %+v", p)
	}

	p, err = ParseMaterial("kl2 k", EitherColour)
	if err != nil {
		t.Fatal(err)
	}
	if p.numMinor[0] != 2 || p.minorOccurs[0] != exactly {
		t.Errorf("L2: %+v", p)
	}

	p, err = ParseMaterial("kp3- kr?", EitherColour)
	if err != nil {
		t.Fatal(err)
	}
	if p.num[0][chess.Pawn] != 3 || p.occ[0][chess.Pawn] != numOrLess || p.num[1][chess.Rook] != 1 || p.occ[1][chess.Rook] != numOrLess {
		t.Errorf("3- and ?: %+v", p)
	}

	p, err = ParseMaterial("kq1>= kn#   trailing comment", EitherColour)
	if err != nil {
		t.Fatal(err)
	}
	if p.occ[0][chess.Queen] != moreEqOpp || p.occ[1][chess.Knight] != notSameAsOpp {
		t.Errorf(">= and #: %+v", p)
	}

	for _, bad := range []string{"kx k", "kp12 k", "k&"} {
		if _, err := ParseMaterial(bad, EitherColour); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func TestReadMaterialFile(t *testing.T) {
	ps, err := ReadMaterialFile(strings.NewReader("% c\nkr k\n\nkq k\r\n"), EitherColour)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 || ps[0].num[0][chess.Rook] != 1 || ps[1].num[0][chess.Queen] != 1 {
		t.Errorf("patterns: %+v", ps)
	}
	if _, err := ReadMaterialFile(strings.NewReader("kr k\nkx k\n"), EitherColour); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("want line 2 error, got %v", err)
	}
}

// rookEnding: Q+R v R; Qxd8+ then Kxd8 leaves K+R v K from ply 2 on.
const rookEnding = "[FEN \"3rk3/8/8/8/8/8/8/3QK2R w K - 0 1\"]\n\n1. Qxd8+ Kxd8 2. Kf1 Kc7 3. Ke1 Kd7 4. Rh2 *\n"

func TestMatchMaterialRookEnding(t *testing.T) {
	g, r := replayed(t, rookEnding)
	p, _ := ParseMaterial("kr k", EitherColour)
	if !matchMaterial(g, r, []*MaterialPattern{p}, "MATCH", true) {
		t.Fatal("R v K should match")
	}
	// R v K first holds after ply 2; depth 2 means it must also hold at
	// plies 3 and 4, so the match lands on ply 4 (Kc7).
	for i, m := range g.Moves {
		want := 0
		if i == 3 {
			want = 1
		}
		if len(m.Comments) != want {
			t.Errorf("ply %d comments = %v, want %d", i+1, m.Comments, want)
		}
	}
	if v, ok := g.Tag("MaterialMatch"); !ok || v != "White" {
		t.Errorf("MaterialMatch tag = %q, %v", v, ok)
	}
}

func TestMatchMaterialDepthAndColours(t *testing.T) {
	g, r := replayed(t, rookEnding)
	p, _ := ParseMaterial("0 kr k", EitherColour)
	if !matchMaterial(g, r, []*MaterialPattern{p}, "M", false) || len(g.Moves[1].Comments) != 1 {
		t.Error("depth 0 matches immediately after Kxd8 (ply 2)")
	}
	// Too deep for the game: 6 half-moves of stability are not available.
	g, r = replayed(t, rookEnding)
	p, _ = ParseMaterial("6 kr k", EitherColour)
	if matchMaterial(g, r, []*MaterialPattern{p}, "M", false) {
		t.Error("game ends before depth 6 is reached")
	}
	// WhiteFirst: "k kr" says Black has the rook, which never holds.
	g, r = replayed(t, rookEnding)
	p, _ = ParseMaterial("0 k kr", WhiteFirst)
	if matchMaterial(g, r, []*MaterialPattern{p}, "", false) {
		t.Error("WhiteFirst with rook on the wrong side")
	}
	g, r = replayed(t, rookEnding)
	p, _ = ParseMaterial("0 k kr", EitherColour)
	if !matchMaterial(g, r, []*MaterialPattern{p}, "", true) {
		t.Error("EitherColour tries both assignments")
	}
	if v, _ := g.Tag("MaterialMatch"); v != "Black" {
		t.Errorf("MaterialMatch = %q, want Black", v)
	}
}

func TestMatchMaterialStartPosition(t *testing.T) {
	g, r := replayed(t, "[FEN \"4k3/8/8/8/8/8/8/4K2R w K - 0 1\"]\n\n1. Rh2 Kd8 2. Rh1 Ke8 *\n")
	p, _ := ParseMaterial("kr k", EitherColour)
	if !matchMaterial(g, r, []*MaterialPattern{p}, "MATCH", false) {
		t.Fatal("should match")
	}
	// Holds at start (depth→1), ply 1 (→2), ply 2 (match): comment on ply 2.
	if len(g.PrefixComments) != 0 || len(g.Moves[1].Comments) != 1 {
		t.Errorf("prefix=%v moves=%v", g.PrefixComments, g.Moves[1].Comments)
	}
	g, r = replayed(t, "[FEN \"4k3/8/8/8/8/8/8/4K2R w K - 0 1\"]\n\n1. Rh2 *\n")
	p, _ = ParseMaterial("0 kr k", EitherColour)
	matchMaterial(g, r, []*MaterialPattern{p}, "MATCH", false)
	if len(g.PrefixComments) != 1 || g.PrefixComments[0] != "MATCH" {
		t.Errorf("start-position match goes to prefix comments: %v", g.PrefixComments)
	}
}

func TestMatchMaterialMinorWildcard(t *testing.T) {
	// White has B+N (two minors), Black lone king.
	g, r := replayed(t, "[FEN \"4k3/8/8/8/8/8/8/2B1KN2 w - - 0 1\"]\n\n1. Kd2 Kd8 2. Ke1 Ke8 *\n")
	p, _ := ParseMaterial("kl2 k", EitherColour)
	if !matchMaterial(g, r, []*MaterialPattern{p}, "", false) {
		t.Error("L2 matches B+N")
	}
	g, r = replayed(t, "[FEN \"4k3/8/8/8/8/8/8/2B1KN2 w - - 0 1\"]\n\n1. Kd2 Kd8 2. Ke1 Ke8 *\n")
	p, _ = ParseMaterial("kb2 k", EitherColour)
	if matchMaterial(g, r, []*MaterialPattern{p}, "", false) {
		t.Error("B2 does not match B+N")
	}
	if !matchMaterial(g, r, nil, "", false) {
		t.Error("no patterns matches everything")
	}
}

func TestMatchPieceCount(t *testing.T) {
	g, r := replayed(t, rookEnding) // 5 pieces, then 4 after Qxd8, 3 after Kxd8
	if !matchPieceCount(g, r, 3, "PC") || len(g.Moves[1].Comments) != 1 {
		t.Errorf("3 pieces after ply 2: %v", g.Moves[1].Comments)
	}
	g, r = replayed(t, rookEnding)
	if !matchPieceCount(g, r, 5, "PC") || len(g.PrefixComments) != 1 {
		t.Error("5 pieces at start → prefix comment")
	}
	g, r = replayed(t, rookEnding)
	if matchPieceCount(g, r, 2, "") {
		t.Error("never 2 pieces")
	}
}
