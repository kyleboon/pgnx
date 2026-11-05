package filter

import "testing"

// recordingSetup is a SetupSeen that records what it is offered and
// answers from a script.
type recordingSetup struct {
	offered []string
	seen    bool
}

func (s *recordingSetup) Seen(fen string) bool {
	s.offered = append(s.offered, fen)
	return s.seen
}

const fenStart = "[SetUp \"1\"]\n[FEN \"rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq - 0 1\"]\n\n1... e5 2. Nf3 *\n"

// TestSameSetupSitsAfterTheTagCriteria: check_duplicate_setup runs after
// check_tag_details_not_ECO and check_setup_tag and before
// apply_move_list (grammar.c deal_with_game), so a game that fails a tag
// criterion is never offered and one that fails a length bound is.
func TestSameSetupSitsAfterTheTagCriteria(t *testing.T) {
	m := &TagMatcher{}
	m.AddPrefix("Player", "Nobody")
	s := &recordingSetup{}
	mustMatch(t, foolsMate, &Options{Tags: m, SameSetup: s}, false)
	if len(s.offered) != 0 {
		t.Errorf("a game failing a tag criterion was offered: %q", s.offered)
	}

	s = &recordingSetup{}
	mustMatch(t, foolsMate, &Options{Bounds: Bounds{MinPly: 100}, SameSetup: s}, false)
	if len(s.offered) != 1 || s.offered[0] != "" {
		t.Errorf("a game failing a length bound: offered %q, want the standard start once", s.offered)
	}

	s = &recordingSetup{}
	mustMatch(t, fenStart, &Options{SameSetup: s}, true)
	if len(s.offered) != 1 || s.offered[0] != "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq - 0 1" {
		t.Errorf("a FEN game: offered %q, want its FEN tag", s.offered)
	}
}

// TestSameSetupSeenStopsTheChain: a seen setup leaves the game unmatched
// and nothing later in the chain runs, so a positional criterion that
// would have matched and labelled the game does not.
func TestSameSetupSeenStopsTheChain(t *testing.T) {
	pc := &PositionCriteria{}
	if err := pc.AddPattern("*/*/*/*/*/*/*/*", false, "L"); err != nil {
		t.Fatal(err)
	}
	o := &Options{Positions: pc, AddLabelTag: true, SameSetup: &recordingSetup{seen: true}}
	g, r := replayed(t, foolsMate)
	ok, err := Evaluate(g, r, o)
	if err != nil || ok {
		t.Fatalf("matched %v err %v, want unmatched", ok, err)
	}
	if _, has := g.Tag("MatchLabel"); has {
		t.Error("a later criterion ran on a game with a seen setup")
	}
	// Not seen: the chain goes on and the label lands.
	o.SameSetup = &recordingSetup{}
	g, r = replayed(t, foolsMate)
	if ok, _ := Evaluate(g, r, o); !ok {
		t.Fatal("unseen setup: want matched")
	}
	if label, _ := g.Tag("MatchLabel"); label != "L" {
		t.Errorf("MatchLabel %q, want L", label)
	}
}
