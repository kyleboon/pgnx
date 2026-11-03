package pipeline

import (
	"strings"
	"testing"

	"github.com/kyleboon/pgnx/transform"
)

// Under SplitVariations a game is emitted as its main line followed by
// one game per variation, each replayed and transformed on its own:
// PlyCount counts the emitted line, while HashCode is the main line's on
// every game (format_game stamps cumulative_hash_value, summed when the
// game was replayed for matching, on each). The oracle:
//
//	pgn-extract --splitvariants --plycount --addhashcode --quiet
func TestProcessSplitVariationsEmissions(t *testing.T) {
	p := New(Config{SplitVariations: true, Transform: &transform.Options{PlyCount: true, HashCode: true}})
	o := p.Process(parse(t, "[Event \"p\"]\n[Result \"*\"]\n\n1. e4 e5 (1... c5) 2. Nf3 *\n"))
	if o.Status != OK || !o.Matched {
		t.Fatalf("status %v matched %v: %v", o.Status, o.Matched, o.Reason)
	}
	want := []struct{ sans, plyCount string }{{"e4 e5 Nf3", "3"}, {"e4 c5", "2"}}
	es := o.Emissions()
	if len(es) != len(want) {
		t.Fatalf("%d emissions, want %d", len(es), len(want))
	}
	for i, e := range es {
		var sans []string
		for _, m := range e.Game.Moves {
			sans = append(sans, m.SAN)
			if len(m.Variations()) > 0 {
				t.Errorf("emission %d: move %s still has variations", i, m.SAN)
			}
		}
		if got := strings.Join(sans, " "); got != want[i].sans {
			t.Errorf("emission %d: moves %q, want %q", i, got, want[i].sans)
		}
		if e.Replay == nil || len(e.Replay.Moves) != len(e.Game.Moves) {
			t.Errorf("emission %d: replay does not cover the game", i)
		}
		if got, _ := e.Game.Tag("PlyCount"); got != want[i].plyCount {
			t.Errorf("emission %d: PlyCount %q, want %q", i, got, want[i].plyCount)
		}
		if got, _ := e.Game.Tag("HashCode"); got != "ea011759" {
			t.Errorf("emission %d: HashCode %q, want ea011759", i, got)
		}
	}
	if o.Game != es[0].Game || o.Replay != es[0].Replay {
		t.Error("Outcome.Game and Replay should be the main line's emission")
	}
}

// Without SplitVariations the one emission is the game and its replay.
func TestProcessEmissionsWithoutSplit(t *testing.T) {
	o := New(Config{}).Process(parse(t, normal))
	es := o.Emissions()
	if len(es) != 1 || es[0].Game != o.Game || es[0].Replay != o.Replay {
		t.Errorf("emissions %+v, want the outcome's game and replay", es)
	}
	if es := New(Config{}).Process(parse(t, badFEN)).Emissions(); es != nil {
		t.Errorf("a Rejected game has emissions: %+v", es)
	}
}

// A Broken game kept with KeepBroken is split too, every game emitted in
// Source format; a variation that does not replay is reported and emitted
// only under KeepBroken, in Source format.
func TestProcessSplitVariationsBrokenAndIllegal(t *testing.T) {
	const brokenMain = "[Event \"b\"]\n\n1. e4 e5 (1... c5) 2. Nf3 Nf3 *\n"
	const illegalVariation = "[Event \"i\"]\n\n1. e4 e5 (1... e4) 2. Nf3 *\n"
	cases := []struct {
		name       string
		src        string
		keepBroken bool
		status     Status
		// emitted lists the SAN of each emission's moves; replayed says
		// which of them carry a replay.
		emitted  []string
		replayed []bool
		warnings int
	}{
		{"broken main line rejected", brokenMain, false, Rejected, nil, nil, 0},
		{"broken main line kept", brokenMain, true, Broken, []string{"e4 e5 Nf3 Nf3", "e4 c5"}, []bool{false, false}, 0},
		{"illegal variation skipped", illegalVariation, false, OK, []string{"e4 e5 Nf3"}, []bool{true}, 1},
		{"illegal variation kept", illegalVariation, true, OK, []string{"e4 e5 Nf3", "e4 e4"}, []bool{true, false}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := New(Config{SplitVariations: true, KeepBroken: c.keepBroken}).Process(parse(t, c.src))
			if o.Status != c.status {
				t.Fatalf("status %v, want %v (%v)", o.Status, c.status, o.Reason)
			}
			es := o.Emissions()
			if len(es) != len(c.emitted) {
				t.Fatalf("%d emissions, want %d", len(es), len(c.emitted))
			}
			for i, e := range es {
				var sans []string
				for _, m := range e.Game.Moves {
					sans = append(sans, m.SAN)
				}
				if got := strings.Join(sans, " "); got != c.emitted[i] {
					t.Errorf("emission %d: %q, want %q", i, got, c.emitted[i])
				}
				if (e.Replay != nil) != c.replayed[i] {
					t.Errorf("emission %d: replay present %v, want %v", i, e.Replay != nil, c.replayed[i])
				}
			}
			if len(o.Warnings) != c.warnings {
				t.Errorf("warnings %v, want %d", o.Warnings, c.warnings)
			}
		})
	}
}
