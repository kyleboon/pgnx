package filter

import (
	"errors"
	"testing"

	"github.com/kyleboon/pgnx/replay"
)

func eval(t *testing.T, src string, o *Options) (bool, error) {
	t.Helper()
	g, r := replayed(t, src)
	return Evaluate(g, r, o)
}

func mustMatch(t *testing.T, src string, o *Options, want bool) {
	t.Helper()
	got, err := eval(t, src, o)
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	if got != want {
		t.Errorf("%q with %+v: matched %v, want %v", src, o, got, want)
	}
}

func TestEvaluateZeroOptions(t *testing.T) {
	mustMatch(t, foolsMate, &Options{}, true)
	mustMatch(t, foolsMate, nil, true)
}

func TestEvaluateResultCriterionSeesPreFixTag(t *testing.T) {
	// Tag says 1-0 but Black mated; --fix-result-tags rewrites it to 0-1,
	// yet -Tr1-0 is tested first (lists.c runs before apply_move_list).
	m := &TagMatcher{}
	m.AddPrefix("Result", "1-0")
	src := "[Result \"1-0\"]\n\n" + foolsMate
	g, r := replayed(t, src)
	ok, err := Evaluate(g, r, &Options{Tags: m, FixResultTags: true})
	if err != nil || !ok {
		t.Fatalf("matched %v err %v", ok, err)
	}
	if tag, _ := g.Tag("Result"); tag != "0-1" || g.Result != "0-1" {
		t.Errorf("after fix: tag %q token %q", tag, g.Result)
	}
	m = &TagMatcher{}
	m.AddPrefix("Result", "0-1")
	mustMatch(t, src, &Options{Tags: m, FixResultTags: true}, false)
}

func TestEvaluateResultNormalisedBeforeTags(t *testing.T) {
	m := &TagMatcher{}
	m.AddPrefix("Result", "1/2-1/2")
	mustMatch(t, "[Result \"1/2\"]\n\n1. e4 1/2-1/2\n", &Options{Tags: m}, true)
	mustMatch(t, "[Result \"?\"]\n\n1. e4 1/2-1/2\n", &Options{Tags: m}, true)
}

func TestEvaluateNoBadResults(t *testing.T) {
	_, err := eval(t, "[Result \"1-0\"]\n\n1. e4 0-1\n", &Options{NoBadResults: true})
	if !errors.Is(err, ErrBadResult) {
		t.Errorf("want ErrBadResult, got %v", err)
	}
	mustMatch(t, "[Result \"1-0\"]\n\n1. e4 *\n", &Options{NoBadResults: true}, true)
}

func TestEvaluateSetup(t *testing.T) {
	fenGame := "[FEN \"4k3/8/8/8/8/8/8/4K2R w K - 0 1\"]\n\n1. Rh2 *\n"
	mustMatch(t, fenGame, &Options{Setup: NoSetup}, false) // SetUp "1" is added first
	mustMatch(t, fenGame, &Options{Setup: OnlySetup}, true)
	mustMatch(t, foolsMate, &Options{Setup: NoSetup}, true)
	mustMatch(t, foolsMate, &Options{Setup: OnlySetup}, false)
	mustMatch(t, "[SetUp \"1\"]\n\n1. e4 *\n", &Options{}, false)
}

func TestEvaluateTagFailureShortCircuits(t *testing.T) {
	m := &TagMatcher{}
	m.AddPrefix("White", "Nobody")
	src := "[FEN \"4k3/8/8/8/8/8/8/4K2R w K - 98 60\"]\n\n60. Kd1 Kd8 61. Ke1 Ke8 *\n"
	g, r := replayed(t, src)
	ok, _ := Evaluate(g, r, &Options{Tags: m, NMoveRule: 50, MarkMatches: "FIFTY"})
	if ok {
		t.Fatal("should fail on tags")
	}
	for _, mv := range g.Moves {
		if len(mv.Comments) != 0 {
			t.Error("no marks after a tag failure")
		}
	}
}

func TestEvaluateMarksSurviveLaterFailure(t *testing.T) {
	// Fifty-move marks are inserted during the replay step; a later
	// --checkmate failure still leaves them in place (as in C).
	src := "[FEN \"4k3/8/8/8/8/8/8/4K2R w K - 98 60\"]\n\n60. Kd1 Kd8 61. Ke1 Ke8 *\n"
	g, r := replayed(t, src)
	ok, _ := Evaluate(g, r, &Options{NMoveRule: 50, MarkMatches: "FIFTY", Checkmate: true})
	if ok {
		t.Fatal("not checkmate")
	}
	if len(g.Moves[3].Comments) != 1 {
		t.Errorf("marks kept: %v", g.Moves[3].Comments)
	}
}

func TestEvaluatePredicates(t *testing.T) {
	mustMatch(t, foolsMate, &Options{Checkmate: true}, true)
	mustMatch(t, "1. e4 e5 *\n", &Options{Checkmate: true}, false)
	mustMatch(t, foolsMate, &Options{Bounds: Bounds{MinPly: 4, MaxPly: 4}}, true)
	mustMatch(t, foolsMate, &Options{Bounds: Bounds{MinPly: 5}}, false)
	mustMatch(t, "[FEN \"7k/8/6Q1/8/8/8/8/K7 w - - 0 1\"]\n\n1. Qf7 *\n", &Options{Stalemate: true}, true)
	mustMatch(t, foolsMate, &Options{Stalemate: true}, false)
	mustMatch(t, "[FEN \"7k/8/8/8/8/8/1r6/K7 w - - 0 1\"]\n\n1. Kxb2 *\n", &Options{Insufficient: true}, true)
	mustMatch(t, "[FEN \"8/P6k/8/8/8/8/8/K7 w - - 0 1\"]\n\n1. a8=R *\n", &Options{Underpromotion: true}, true)
	mustMatch(t, foolsMate, &Options{Underpromotion: true}, false)
	mustMatch(t, "1. e4 {hi} *\n", &Options{Commented: true}, true)
	mustMatch(t, foolsMate, &Options{Commented: true}, false)
	mustMatch(t, "1. Nf3 Nf6 2. Ng1 Ng8 3. Nf3 Nf6 4. Ng1 Ng8 *\n", &Options{Repetition: 3}, true)
	mustMatch(t, "1. Nf3 Nf6 2. Ng1 Ng8 3. Nf3 Nf6 4. Ng1 Ng8 *\n", &Options{Repetition: 5}, false)
	mustMatch(t, "[FEN \"4k3/8/8/8/8/8/8/4K2R w K - 98 60\"]\n\n60. Kd1 Kd8 61. Ke1 Ke8 *\n", &Options{NMoveRule: 50}, true)
	mustMatch(t, foolsMate, &Options{NMoveRule: 50}, false)
	mustMatch(t, rookEnding, &Options{PieceCount: 3}, true)
	mustMatch(t, rookEnding, &Options{PieceCount: 2}, false)
	p, _ := ParseMaterial("kr k", EitherColour)
	mustMatch(t, rookEnding, &Options{Material: []*MaterialPattern{p}}, true)
	mustMatch(t, foolsMate, &Options{Material: []*MaterialPattern{p}}, false)
	mustMatch(t, "[WhiteElo \"2700\"]\n[BlackElo \"2500\"]\n[Result \"1-0\"]\n\n1. e4 1-0\n", &Options{HigherRatedWinner: true}, true)
	mustMatch(t, "[WhiteElo \"2700\"]\n[BlackElo \"2500\"]\n[Result \"1-0\"]\n\n1. e4 1-0\n", &Options{LowerRatedWinner: true}, false)
	eco := &TagMatcher{}
	eco.AddPrefix("ECO", "B")
	mustMatch(t, "[ECO \"B90\"]\n\n1. e4 *\n", &Options{Tags: eco}, true)
	mustMatch(t, "[ECO \"C90\"]\n\n1. e4 *\n", &Options{Tags: eco}, false)
	mustMatch(t, "1. e4 *\n", &Options{Tags: eco}, false)
}

func TestEvaluateInvalidResultTagBecomesStar(t *testing.T) {
	g, r := replayed(t, "[Result \"1:0\"]\n\n1. e4 *\n")
	if ok, err := Evaluate(g, r, &Options{}); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if v, _ := g.Tag("Result"); v != "*" {
		t.Errorf("Result = %q", v)
	}
}

func TestEvaluatePartialResult(t *testing.T) {
	// A broken game kept with --keep-broken is evaluated against the
	// moves that did replay.
	g, _ := replayed(t, "1. e4 e5 *\n")
	g.Moves = append(g.Moves, g.Moves[0]) // now "1. e4 e5 2. e4": third move illegal
	r, err := replay.Game(g, replay.Options{})
	if err == nil {
		t.Fatal("expected replay failure")
	}
	ok, err := Evaluate(g, r, &Options{Bounds: Bounds{MinPly: 2, MaxPly: 2}})
	if err != nil || !ok {
		t.Errorf("partial: %v %v", ok, err)
	}
	ok, _ = Evaluate(g, nil, &Options{Bounds: Bounds{MinPly: 1}})
	if ok {
		t.Error("nil result has ply count 0")
	}
}

func TestEvaluatePhase3Order(t *testing.T) {
	// A positional match is marked during the replay step; failing the ply
	// bounds afterwards rejects the game but leaves the mark in place, so
	// an unmatched-games output sees it (as in C).
	pc := &PositionCriteria{}
	if err := pc.AddLine("e4 c5"); err != nil {
		t.Fatal(err)
	}
	g, r := replayed(t, sicilianEnglishAttack)
	ok, err := Evaluate(g, r, &Options{Positions: pc, MarkMatches: "POS", Bounds: Bounds{MinPly: 100}})
	if err != nil || ok {
		t.Fatalf("matched %v err %v", ok, err)
	}
	if got := commentsAt(g); len(got) != 1 || got[0] != 1 || g.Moves[1].Comments[0] != "POS" {
		t.Errorf("position mark not kept: comments at %v", got)
	}

	// Bounds are tested before sequences: a game failing bounds is
	// rejected whatever the sequence says; a game failing the sequence is
	// not tested for material, so no material mark is inserted.
	seqs := []*Sequence{mustSeq(t, "d4 d5")}
	mp, err := ParseMaterial("q*r*b*n*p* q*r*b*n*p*", EitherColour)
	if err != nil {
		t.Fatal(err)
	}
	g, r = replayed(t, sicilianEnglishAttack)
	ok, err = Evaluate(g, r, &Options{Sequences: seqs, Material: []*MaterialPattern{mp}, MarkMatches: "MAT"})
	if err != nil || ok {
		t.Fatalf("matched %v err %v", ok, err)
	}
	for _, m := range g.Moves {
		for _, c := range m.Comments {
			if c == "MAT" {
				t.Fatal("material mark inserted although the sequence check failed")
			}
		}
	}
	if _, has := g.Tag("MaterialMatch"); has {
		t.Error("MaterialMatch tag added although the sequence check failed")
	}

	// ECO criteria run last: a game failing only its ECO criterion has
	// already been marked for its material match and tagged.
	eco := &TagMatcher{}
	eco.AddPrefix("ECO", "D")
	g, r = replayed(t, "[ECO \"B90\"]\n\n"+sicilianEnglishAttack)
	ok, err = Evaluate(g, r, &Options{Tags: eco, Material: []*MaterialPattern{mp}, MarkMatches: "MAT", AddMatchTag: true})
	if err != nil || ok {
		t.Fatalf("matched %v err %v", ok, err)
	}
	if len(commentsAt(g)) == 0 {
		t.Error("material mark missing: ECO criteria must be tested after material")
	}
	if _, has := g.Tag("MaterialMatch"); !has {
		t.Error("MaterialMatch tag missing: ECO criteria must be tested after material")
	}
	g, r = replayed(t, "[ECO \"D90\"]\n\n"+sicilianEnglishAttack)
	if ok, _ := Evaluate(g, r, &Options{Tags: eco, Material: []*MaterialPattern{mp}}); !ok {
		t.Error("ECO criterion satisfied: want match")
	}
}
