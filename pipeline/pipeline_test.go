package pipeline

import (
	"strings"
	"sync"
	"testing"

	"github.com/kyleboon/pgnx/eco"
	"github.com/kyleboon/pgnx/filter"
	"github.com/kyleboon/pgnx/pgn"
)

// parse returns the single game in src, parsed as the CLI parses it.
func parse(t *testing.T, src string) *pgn.Game {
	t.Helper()
	s := pgn.NewScanner(strings.NewReader(src), pgn.ParserOptions{SkipTagsOnly: true})
	if !s.Next() {
		t.Fatalf("no game in %q: %v", src, s.Err())
	}
	g := s.Game()
	if s.Next() {
		t.Fatalf("more than one game in %q", src)
	}
	return g
}

// ecoTable is a two-line ECO file: the Reti (1. Nf3) and its 1... d5 line.
func ecoTable(t *testing.T) *eco.Table {
	t.Helper()
	const src = "[ECO \"A04\"]\n[Opening \"Reti opening\"]\n\n1. Nf3 *\n\n" +
		"[ECO \"A06\"]\n[Opening \"Reti opening\"]\n\n1. Nf3 d5 *\n"
	table, err := eco.Load(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	return table
}

const (
	normal         = "[Event \"n\"]\n\n1. f3 e5 2. g4 Qh4# 0-1\n"
	setUpNoFEN     = "[Event \"a\"]\n[SetUp \"1\"]\n\n1. e4 *\n"
	badFEN         = "[Event \"b\"]\n[FEN \"garbage\"]\n\n1. e4 *\n"
	broken         = "[Event \"c\"]\n\n1. e4 e4 *\n"
	noResultBroken = "[Event \"d\"]\n\n1. e4 e4\n"
	noResultOK     = "[Event \"d2\"]\n\n1. e4 e5\n"
	stray          = "[Event \"e\"]\n\n1. e4 e5 2. N Nf3 *\n"
	retiD5         = "[Event \"f\"]\n[ECO \"B00\"]\n\n1. Nf3 d5 2. c4 *\n"
	badResultTag   = "[Event \"g\"]\n[Result \"1:0\"]\n\n1. e4 1-0\n"
)

// Process runs pgn-extract's deal_with_game sequence over one game:
// normalise, replay (dropping the moves of a result-less game that does
// not replay, as the C parser never replays them), decide broken-ness
// against keep-broken, classify, evaluate the criteria. The table pins
// the outcome of each edge game under each configuration.
func TestProcessEdgeGames(t *testing.T) {
	cases := []struct {
		name        string
		src         string
		cfg         Config
		status      Status
		reason      string // substring of Reason, "" for nil
		warnings    int
		matched     bool
		moves       int // len(Game.Moves) after processing
		replayed    int // len(Replay.Moves); -1 for a nil Replay
		resultTag   string
		ecoTag      string
		wantSkipped int
	}{
		{name: "normal game", src: normal, status: OK, matched: true, moves: 4, replayed: 4, resultTag: "0-1"},
		{name: "normal game with keep-broken", src: normal, cfg: Config{KeepBroken: true}, status: OK, matched: true, moves: 4, replayed: 4, resultTag: "0-1"},
		{name: "SetUp without FEN", src: setUpNoFEN, status: Rejected, reason: "SetUp", moves: 1, replayed: -1, resultTag: "*"},
		{name: "SetUp without FEN with keep-broken", src: setUpNoFEN, cfg: Config{KeepBroken: true}, status: Rejected, reason: "SetUp", moves: 1, replayed: -1, resultTag: "*"},
		{name: "invalid FEN", src: badFEN, status: Rejected, reason: `"garbage"`, moves: 1, replayed: -1, resultTag: "*"},
		{name: "invalid FEN with keep-broken", src: badFEN, cfg: Config{KeepBroken: true}, status: Rejected, reason: `"garbage"`, moves: 1, replayed: -1, resultTag: "*"},
		{name: "broken game", src: broken, status: Rejected, reason: "illegal move 2", moves: 2, replayed: -1, resultTag: "*"},
		{name: "broken game with keep-broken", src: broken, cfg: Config{KeepBroken: true}, status: Broken, reason: "illegal move 2", matched: true, moves: 2, replayed: 1, resultTag: "*"},
		{name: "missing result token and unreplayable moves", src: noResultBroken, status: OK, warnings: 1, matched: true, moves: 0, replayed: 0, resultTag: ""},
		{name: "missing result token and unreplayable moves with keep-broken", src: noResultBroken, cfg: Config{KeepBroken: true}, status: Broken, reason: "illegal move 2", matched: true, moves: 2, replayed: 1, resultTag: ""},
		{name: "missing result token and good moves", src: noResultOK, status: OK, matched: true, moves: 2, replayed: 2, resultTag: ""},
		{name: "stray movetext token", src: stray, status: OK, matched: true, moves: 3, replayed: 3, resultTag: "*", wantSkipped: 1},
		{name: "ECO-classified game", src: retiD5, cfg: Config{ECO: ecoTable(t)}, status: OK, matched: true, moves: 3, replayed: 3, resultTag: "*", ecoTag: "A06"},
		{name: "ECO-classified broken game", src: "[Event \"h\"]\n\n1. Nf3 d5 2. Qh5 *\n", cfg: Config{ECO: ecoTable(t), KeepBroken: true}, status: Broken, reason: "illegal move 3", matched: true, moves: 3, replayed: 2, resultTag: "*", ecoTag: "A06"},
		{name: "invalid Result tag becomes * even with no criteria", src: badResultTag, status: OK, matched: true, moves: 1, replayed: 1, resultTag: "*"},
		{name: "criteria unmatched", src: normal, cfg: Config{Criteria: &filter.Options{Checkmate: true, Bounds: filter.Bounds{MinPly: 10}}}, status: OK, matched: false, moves: 4, replayed: 4, resultTag: "0-1"},
		{name: "criteria matched", src: normal, cfg: Config{Criteria: &filter.Options{Checkmate: true}}, status: OK, matched: true, moves: 4, replayed: 4, resultTag: "0-1"},
		{name: "broken game evaluated against the partial replay", src: broken, cfg: Config{KeepBroken: true, Criteria: &filter.Options{Bounds: filter.Bounds{MinPly: 2}}}, status: Broken, reason: "illegal move 2", matched: false, moves: 2, replayed: 1, resultTag: "*"},
		{name: "bad result rejected on request", src: "[Event \"i\"]\n[Result \"1-0\"]\n\n1. e4 0-1\n", cfg: Config{Criteria: &filter.Options{NoBadResults: true}}, status: Rejected, reason: filter.ErrBadResult.Error(), moves: 1, replayed: -1, resultTag: "1-0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := parse(t, c.src)
			o := New(c.cfg).Process(g)
			if o.Game != g {
				t.Errorf("Outcome.Game is not the processed game")
			}
			if o.Status != c.status {
				t.Errorf("Status = %v, want %v (reason %v)", o.Status, c.status, o.Reason)
			}
			switch {
			case c.reason == "" && o.Reason != nil:
				t.Errorf("Reason = %v, want nil", o.Reason)
			case c.reason != "" && (o.Reason == nil || !strings.Contains(o.Reason.Error(), c.reason)):
				t.Errorf("Reason = %v, want one containing %q", o.Reason, c.reason)
			}
			if (o.Reason == nil) != (o.Status == OK) {
				t.Errorf("Reason nil iff OK violated: status %v reason %v", o.Status, o.Reason)
			}
			if len(o.Warnings) != c.warnings {
				t.Errorf("Warnings = %v, want %d", o.Warnings, c.warnings)
			}
			if o.Matched != c.matched {
				t.Errorf("Matched = %v, want %v", o.Matched, c.matched)
			}
			if len(g.Moves) != c.moves {
				t.Errorf("moves kept = %d, want %d", len(g.Moves), c.moves)
			}
			replayed := -1
			if o.Replay != nil {
				replayed = len(o.Replay.Moves)
			}
			if replayed != c.replayed {
				t.Errorf("replayed plies = %d, want %d", replayed, c.replayed)
			}
			if tag, _ := g.Tag("Result"); tag != c.resultTag {
				t.Errorf("Result tag = %q, want %q", tag, c.resultTag)
			}
			if tag, _ := g.Tag("ECO"); tag != c.ecoTag {
				t.Errorf("ECO tag = %q, want %q", tag, c.ecoTag)
			}
			if len(g.SkippedTokens()) != c.wantSkipped {
				t.Errorf("skipped tokens = %v, want %d (a parse fact, never a Warning)", g.SkippedTokens(), c.wantSkipped)
			}
		})
	}
}

func TestProcessMissingResultWarningNamesTheFailure(t *testing.T) {
	o := New(Config{}).Process(parse(t, noResultBroken))
	if len(o.Warnings) != 1 {
		t.Fatalf("warnings: %v", o.Warnings)
	}
	w := o.Warnings[0].Error()
	if !strings.Contains(w, "no result token") || !strings.Contains(w, "illegal move 2") || !strings.Contains(w, "dropped") {
		t.Errorf("warning = %q", w)
	}
}

// A nil Criteria is the empty criteria: filter with no criteria is
// convert by construction.
func TestProcessNilCriteriaMatchesAll(t *testing.T) {
	for _, cfg := range []Config{{}, {Criteria: &filter.Options{}}} {
		o := New(cfg).Process(parse(t, normal))
		if o.Status != OK || !o.Matched {
			t.Errorf("%+v: status %v matched %v", cfg, o.Status, o.Matched)
		}
	}
}

func TestStatusString(t *testing.T) {
	for s, want := range map[Status]string{OK: "ok", Broken: "broken", Rejected: "rejected", Status(9): "Status(9)"} {
		if got := s.String(); got != want {
			t.Errorf("%d: %q, want %q", int(s), got, want)
		}
	}
}

// Process touches only its game and read-only configuration, so one
// Pipeline may process different games concurrently (ADR 0002; the
// future --jobs). Run under -race.
func TestProcessConcurrent(t *testing.T) {
	cfg := Config{
		ECO:        ecoTable(t),
		KeepBroken: true,
		Criteria:   &filter.Options{Bounds: filter.Bounds{MinPly: 1}},
	}
	p := New(cfg)
	srcs := []string{normal, broken, noResultBroken, stray, retiD5, badResultTag, setUpNoFEN}
	const rounds = 20
	var wg sync.WaitGroup
	outcomes := make([]Outcome, len(srcs)*rounds)
	for i := range outcomes {
		g := parse(t, srcs[i%len(srcs)])
		wg.Add(1)
		go func(i int, g *pgn.Game) {
			defer wg.Done()
			outcomes[i] = p.Process(g)
		}(i, g)
	}
	wg.Wait()
	for i, o := range outcomes {
		want := New(cfg).Process(parse(t, srcs[i%len(srcs)]))
		if o.Status != want.Status || o.Matched != want.Matched || len(o.Warnings) != len(want.Warnings) ||
			(o.Reason == nil) != (want.Reason == nil) || len(o.Game.Tags) != len(want.Game.Tags) {
			t.Errorf("game %d (%q): concurrent outcome differs: %+v vs %+v", i, srcs[i%len(srcs)], o, want)
		}
	}
}
