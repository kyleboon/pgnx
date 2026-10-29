package filter

import (
	"errors"
	"strings"
	"testing"
)

func mustSeq(t *testing.T, line string) *Sequence {
	t.Helper()
	s, err := ParseSequence(line)
	if err != nil {
		t.Fatalf("ParseSequence(%q): %v", line, err)
	}
	if s == nil {
		t.Fatalf("ParseSequence(%q): nil sequence", line)
	}
	return s
}

func TestParseSequence(t *testing.T) {
	tests := []struct {
		line                   string
		moves                  []string
		wAny, bAny, wDis, bDis int
	}{
		{"1. e4 c5 2. Nf3", []string{"e4", "c5", "Nf3"}, 0, 0, 0, 0},
		{"1.e4 c5 2.Nf3 d6 3.d4", []string{"e4", "c5", "Nf3", "d6", "d4"}, 0, 0, 0, 0},
		{"1.e4 1...c5", []string{"e4", "c5"}, 0, 0, 0, 0},
		{"* b6", []string{"*", "b6"}, 1, 0, 0, 0},
		{"e4 c5 !Nf3", []string{"e4", "c5", "!Nf3"}, 0, 0, 1, 0},
		{"e4 !c5", []string{"e4", "!c5"}, 0, 0, 0, 1},
		{"e4 c5 Nf3 d6 d4 cxd|cxd4 Nxd4 * Nc3 * Be3",
			[]string{"e4", "c5", "Nf3", "d6", "d4", "cxd|cxd4", "Nxd4", "*", "Nc3", "*", "Be3"}, 0, 2, 0, 0},
		{"  c4\tNf6   Nc3 ", []string{"c4", "Nf6", "Nc3"}, 0, 0, 0, 0},
	}
	for _, tc := range tests {
		s := mustSeq(t, tc.line)
		var got []string
		for _, m := range s.moves {
			got = append(got, m.text)
		}
		if strings.Join(got, " ") != strings.Join(tc.moves, " ") {
			t.Errorf("%q: moves %q, want %q", tc.line, got, tc.moves)
		}
		if s.whiteAny != tc.wAny || s.blackAny != tc.bAny || s.whiteDis != tc.wDis || s.blackDis != tc.bDis {
			t.Errorf("%q: counts any %d/%d dis %d/%d, want any %d/%d dis %d/%d", tc.line,
				s.whiteAny, s.blackAny, s.whiteDis, s.blackDis, tc.wAny, tc.bAny, tc.wDis, tc.bDis)
		}
		if s.Len() != len(tc.moves) {
			t.Errorf("%q: Len %d, want %d", tc.line, s.Len(), len(tc.moves))
		}
	}
	for _, blank := range []string{"", "   ", "\r", "% e4 c5"} {
		s, err := ParseSequence(blank)
		if s != nil || err != nil {
			t.Errorf("ParseSequence(%q) = %v, %v; want nil, nil", blank, s, err)
		}
	}
	if _, err := ParseSequence("1. 2."); !errors.Is(err, ErrNoMoves) {
		t.Errorf("number-only line: err %v, want ErrNoMoves", err)
	}
}

func TestReadSequences(t *testing.T) {
	in := "% comment\nc4 Nf6 Nc3 e6 d4 Bb4\n\n1. e4 c5 2. Nf3\r\n"
	seqs, err := ReadSequences(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(seqs) != 2 || seqs[0].Len() != 6 || seqs[1].Len() != 3 {
		t.Fatalf("got %d sequences: %+v", len(seqs), seqs)
	}
	if _, err := ReadSequences(strings.NewReader("e4\n12.\n")); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("bad line: err %v", err)
	}
}

func TestTextualMatch(t *testing.T) {
	tests := []struct {
		variation, actual string
		want              bool
	}{
		{"Nc6|Nf3|f3", "f3", true},
		{"Nc6|Nf3|f3", "c6", false},
		{"Nc6|Nf3|f3", "Nc6", true},
		{"Nc6|Nf3|f3", "Nf3", true},
		{"Nc6|Nf3|f3", "N", false},
		{"!Nf3", "Nf3", true},
		{"cxd|cxd4", "cxd4", true},
		{"cxd|cxd4", "cxd", true},
		{"O-O", "O-O", true},
		{"O-O-O", "O-O", false},
		{"e4", "e4", true},
		{"e4", "e5", false},
		{"*", "e4", false},
		{"*|c4", "c4", true},
		{"e4", "", false},
	}
	for _, tc := range tests {
		if got := textualMatch(tc.variation, tc.actual); got != tc.want {
			t.Errorf("textualMatch(%q, %q) = %v, want %v", tc.variation, tc.actual, got, tc.want)
		}
	}
}

func TestStraightAndPermutation(t *testing.T) {
	pvars := mustSeq(t, "c4 Nf6 Nc3 e6 d4 Bb4")
	ordered := strings.Fields("c4 Nf6 Nc3 e6 d4 Bb4 Qc2 O-O")
	transposed := strings.Fields("d4 Nf6 c4 e6 Nc3 Bb4 Qc2 O-O")
	if !pvars.straight(ordered) {
		t.Error("straight: ordered line should match")
	}
	if pvars.straight(transposed) {
		t.Error("straight: transposed line should not match")
	}
	if !pvars.permutation(ordered) || !pvars.permutation(transposed) {
		t.Error("permutation: both orders should match")
	}
	// Colours are respected: Black's e6 played by White (index parity) is not a match.
	wrongColour := strings.Fields("e6 c4 Nf6 Nc3 d4 Bb4")
	if pvars.permutation(wrongColour) {
		t.Error("permutation: moves of the wrong colour should not match")
	}
	// Game shorter than the sequence.
	short := strings.Fields("c4 Nf6 Nc3")
	if pvars.straight(short) || pvars.permutation(short) {
		t.Error("a game shorter than the sequence must not match")
	}
	if pvars.straight(nil) || pvars.permutation(nil) {
		t.Error("an empty game must not match")
	}
}

func TestDisallowedMoves(t *testing.T) {
	s := mustSeq(t, "e4 c5 !Nf3")
	open := strings.Fields("e4 c5 Nc3 Nc6 g3")
	nf3 := strings.Fields("e4 c5 Nf3 d6")
	if !s.straight(open) || !s.permutation(open) {
		t.Error("game without 2.Nf3 should match both ways")
	}
	if s.straight(nf3) || s.permutation(nf3) {
		t.Error("game with 2.Nf3 should fail both ways")
	}
	// Straight only looks at index 2; permutation scans the whole window
	// for a disallowed move of the right colour.
	nf3Later := strings.Fields("Nf3 c5 e4 d6")
	if !s.straight(mustFields("e4 c5 d4")) {
		t.Error("straight sanity")
	}
	if s.permutation(nf3Later) {
		t.Error("permutation: Nf3 anywhere in White's window is disallowed")
	}
	// A disallowed Black move does not rule out the same move by White.
	b := mustSeq(t, "e4 !Nf6")
	if !b.permutation(strings.Fields("e4 e5 Nf3 Nf6")) {
		t.Error("permutation: disallowed Black move only scanned within the 2-ply window")
	}
	if b.permutation(strings.Fields("e4 Nf6")) || b.straight(strings.Fields("e4 Nf6")) {
		t.Error("1...Nf6 is disallowed")
	}
	// Surviving disallowed entries become any-move slack in stage two.
	if !b.straight(strings.Fields("e4 e5")) || !b.permutation(strings.Fields("e4 e5")) {
		t.Error("e4 e5 should satisfy e4 !Nf6")
	}
	// Game shorter than the sequence still fails even when no disallowed
	// move was found.
	if b.permutation(strings.Fields("e4")) || b.straight(strings.Fields("e4")) {
		t.Error("too short")
	}
}

func mustFields(s string) []string { return strings.Fields(s) }

func TestAnyMoveSlack(t *testing.T) {
	// vvars.txt from the C test suite: 7...a6 8.Be3 is the only shape that
	// consumes the two Black any-moves.
	s := mustSeq(t, "e4 c5 Nf3 d6 d4 cxd4 Nxd4 * Nc3 * Be3")
	najdorf := strings.Fields("e4 c5 Nf3 d6 d4 cxd4 Nxd4 Nf6 Nc3 a6 Be3 e5")
	if !s.straight(najdorf) || !s.permutation(najdorf) {
		t.Error("Najdorf English attack should match")
	}
	// Permutation: slack is per colour. Two unknown Black moves plus a
	// known Black move out of order.
	perm := strings.Fields("e4 c5 Nf3 d6 d4 cxd4 Nxd4 Nf6 Be3 a6 Nc3 e5")
	if s.straight(perm) {
		t.Error("straight: Be3 before Nc3 should not match")
	}
	if !s.permutation(perm) {
		t.Error("permutation: Be3/Nc3 swapped should match")
	}
	// An unlisted White move cannot borrow Black's slack.
	whiteOdd := strings.Fields("e4 c5 Nf3 d6 d4 cxd4 Nxd4 Nf6 Nc3 a6 h3 e5")
	if s.permutation(whiteOdd) {
		t.Error("permutation: no White slack for 8.h3")
	}
	// White slack.
	w := mustSeq(t, "* b6")
	if !w.straight(strings.Fields("Nf3 b6")) || !w.permutation(strings.Fields("Nf3 b6")) {
		t.Error("* b6 should match 1.Nf3 b6")
	}
	if w.straight(strings.Fields("Nf3 b5")) || w.permutation(strings.Fields("Nf3 b5")) {
		t.Error("* b6 should not match 1.Nf3 b5")
	}
}

func TestAlternatives(t *testing.T) {
	s := mustSeq(t, "e4 c5 Nf3 Nc6|d6|e6")
	for _, b := range []string{"Nc6", "d6", "e6"} {
		if !s.straight(strings.Fields("e4 c5 Nf3 " + b)) {
			t.Errorf("alternative %s should match", b)
		}
	}
	if s.straight(strings.Fields("e4 c5 Nf3 a6")) {
		t.Error("a6 is not an alternative")
	}
}

const sicilianEnglishAttack = "1. e4 c5 2. Nf3 d6 3. d4 cxd4 4. Nxd4 Nf6 5. Nc3 a6 6. Be3 e5 7. Nb3 Be6 *\n"

func TestSequenceText(t *testing.T) {
	tests := map[string]string{
		"e4": "e4", "e4!?": "e4", "Bb5+": "Bb5+", "Qxf7#": "Qxf7+", "Bxc6+#": "Bxc6+",
		"Qh5++": "Qh5+", "Nf3+!": "Nf3+", "Ngf3": "Ngf3", "O-O": "O-O", "e8=Q#": "e8=Q+",
	}
	for in, want := range tests {
		if got := sequenceText(in); got != want {
			t.Errorf("sequenceText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchSequencesAnywhere(t *testing.T) {
	g, _ := replayed(t, sicilianEnglishAttack)
	seqs := []*Sequence{mustSeq(t, "Nc3 a6 Be3")}
	if matchSequences(g, seqs, true, false) || matchSequences(g, seqs, false, false) {
		t.Error("mid-game sequence must not match from the start")
	}
	if !matchSequences(g, seqs, true, true) || !matchSequences(g, seqs, false, true) {
		t.Error("mid-game sequence should match anywhere")
	}
	// Anywhere stops once no sequence fits in the remaining moves.
	long := []*Sequence{mustSeq(t, "Be6 a3 b4 c4 d5 e6 f7 g8 h1 a2 b3 c5 d6 e7 f8")}
	if matchSequences(g, long, false, true) {
		t.Error("sequence longer than the game never matches")
	}
	// No sequences: everything matches.
	if !matchSequences(g, nil, false, false) {
		t.Error("nil sequences should match")
	}
	// Any of several sequences suffices.
	many := []*Sequence{mustSeq(t, "d4 d5"), mustSeq(t, "e4 c5")}
	if !matchSequences(g, many, true, false) {
		t.Error("second sequence should match")
	}
}

// The oracle matches the moves as written (verified against pgn-extract
// on crafted games): "Ngf3" is not "Nf3"; a written "#" or "+" becomes
// "+" and must be present in the sequence; an unwritten check is matched
// by a sequence move with or without "+".
func TestMatchSequencesWrittenText(t *testing.T) {
	cases := []struct {
		game, seq string
		want      bool
	}{
		{"1. e4 e5 2. Ngf3 Nc6 *", "e4 e5 Nf3", false},
		{"1. e4 e5 2. Ngf3 Nc6 *", "e4 e5 Ngf3", true},
		{"1. e4 e5 2. Qh5 Nc6 3. Bc4 Nf6 4. Qxf7# 1-0", "e4 e5 Qh5 Nc6 Bc4 Nf6 Qxf7", false},
		{"1. e4 e5 2. Qh5 Nc6 3. Bc4 Nf6 4. Qxf7# 1-0", "e4 e5 Qh5 Nc6 Bc4 Nf6 Qxf7#", false},
		{"1. e4 e5 2. Qh5 Nc6 3. Bc4 Nf6 4. Qxf7# 1-0", "e4 e5 Qh5 Nc6 Bc4 Nf6 Qxf7+", true},
		{"1. e4 e5 2. Qh5 Nc6 3. Bc4 Nf6 4. Qxf7+ 1-0", "e4 e5 Qh5 Nc6 Bc4 Nf6 Qxf7+", true},
		{"1. e4 d5 2. Bb5+ c6 *", "e4 d5 Bb5", false},
		{"1. e4 d5 2. Bb5+ c6 *", "e4 d5 Bb5+", true},
		{"1. e4 d5 2. Bb5 c6 *", "e4 d5 Bb5", true},
		{"1. e4 d5 2. Bb5 c6 *", "e4 d5 Bb5+", true},
		{"1. e4!? e5?! 2. Nf3! Nc6 *", "e4 e5 Nf3", true},
	}
	for _, tc := range cases {
		g, _ := replayed(t, tc.game+"\n")
		seqs := []*Sequence{mustSeq(t, tc.seq)}
		for _, exact := range []bool{false, true} {
			if got := matchSequences(g, seqs, exact, false); got != tc.want {
				t.Errorf("%q vs %q exact=%v: %v, want %v", tc.game, tc.seq, exact, got, tc.want)
			}
		}
	}
}

func TestEvaluateSequencesBetweenBoundsAndMaterial(t *testing.T) {
	seqs := []*Sequence{mustSeq(t, "c4 Nf6 Nc3 e6 d4 Bb4")}
	nimzo := "1. d4 Nf6 2. c4 e6 3. Nc3 Bb4 4. Qc2 O-O *\n"
	mustMatch(t, nimzo, &Options{Sequences: seqs}, true)
	mustMatch(t, nimzo, &Options{Sequences: seqs, ExactOrder: true}, false)
	mustMatch(t, "1. c4 Nf6 2. Nc3 e6 3. d4 Bb4 *\n", &Options{Sequences: seqs, ExactOrder: true}, true)
	mustMatch(t, sicilianEnglishAttack, &Options{Sequences: seqs}, false)

	// Bounds are tested before sequences; material after. A game that
	// fails the sequence must not receive a material mark.
	mp, err := ParseMaterial("q*r*b*n*p* q*r*b*n*p*", EitherColour)
	if err != nil {
		t.Fatal(err)
	}
	mustMatch(t, sicilianEnglishAttack, &Options{Material: []*MaterialPattern{mp}}, true)
	g, r := replayed(t, sicilianEnglishAttack)
	ok, err := Evaluate(g, r, &Options{
		Sequences:   seqs,
		Material:    []*MaterialPattern{mp},
		MarkMatches: "MAT",
	})
	if err != nil || ok {
		t.Fatalf("matched %v err %v", ok, err)
	}
	for _, m := range g.Moves {
		for _, c := range m.Comments {
			if strings.Contains(c, "MAT") {
				t.Fatal("material mark inserted although the sequence check failed")
			}
		}
	}
	// A game failing bounds is rejected whatever the sequence says.
	mustMatch(t, nimzo, &Options{Sequences: seqs, Bounds: Bounds{MinPly: 100}}, false)
	// Anywhere through Options.
	mid := []*Sequence{mustSeq(t, "Nc3 a6 Be3")}
	mustMatch(t, sicilianEnglishAttack, &Options{Sequences: mid}, false)
	mustMatch(t, sicilianEnglishAttack, &Options{Sequences: mid, SequenceAnywhere: true}, true)
}
