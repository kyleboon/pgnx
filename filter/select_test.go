package filter

import (
	"reflect"
	"testing"
)

func TestParseRanges(t *testing.T) {
	got, err := ParseRanges("2:4,7")
	if err != nil || !reflect.DeepEqual(got, []Range{{2, 4}, {7, 7}}) {
		t.Errorf("2:4,7 = %v, %v", got, err)
	}
	got, err = ParseRanges("2,3")
	if err != nil || !reflect.DeepEqual(got, []Range{{2, 2}, {3, 3}}) {
		t.Errorf("2,3 = %v, %v", got, err)
	}
	for _, bad := range []string{"3,2", "0", "4:2", "2:2,2", "x", "", "1:"} {
		if _, err := ParseRanges(bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

type step struct {
	index   int
	matched bool
	want    Decision
}

func runSteps(t *testing.T, name string, o SelectorOptions, steps []step) {
	t.Helper()
	s := NewSelector(o)
	for _, st := range steps {
		got := s.Decide(st.index, st.matched)
		if got != st.want {
			t.Errorf("%s: game %d matched=%v: got %+v, want %+v", name, st.index, st.matched, got, st.want)
		}
		if st.want.Stop {
			break
		}
	}
}

func TestSelectorPlain(t *testing.T) {
	runSteps(t, "plain", SelectorOptions{}, []step{
		{1, true, Decision{Emit: true}},
		{2, false, Decision{ToUnmatched: true}},
		{3, true, Decision{Emit: true}},
	})
}

func TestSelectorSelectOnly(t *testing.T) {
	// test-selectonly: matches 1..4 → emit 2 and 3 only, stop after 3.
	ranges, _ := ParseRanges("2,3")
	runSteps(t, "selectonly", SelectorOptions{SelectOnly: ranges}, []step{
		{1, true, Decision{}},
		{2, false, Decision{ToUnmatched: true}},
		{3, true, Decision{Emit: true}},
		{4, true, Decision{Emit: true, Stop: true}},
	})
	ranges, _ = ParseRanges("1:2,4")
	runSteps(t, "selectonly ranges", SelectorOptions{SelectOnly: ranges}, []step{
		{1, true, Decision{Emit: true}},
		{2, true, Decision{Emit: true}},
		{3, true, Decision{}},
		{4, true, Decision{Emit: true, Stop: true}},
	})
}

func TestSelectorSkipMatching(t *testing.T) {
	ranges, _ := ParseRanges("1,2")
	runSteps(t, "skipmatching", SelectorOptions{SkipMatching: ranges}, []step{
		{1, true, Decision{}},
		{2, false, Decision{ToUnmatched: true}},
		{3, true, Decision{}},
		{4, true, Decision{Emit: true}},
		{5, true, Decision{Emit: true}},
	})
}

func TestSelectorStopAfter(t *testing.T) {
	runSteps(t, "stopafter", SelectorOptions{StopAfter: 2}, []step{
		{1, true, Decision{Emit: true}},
		{2, false, Decision{ToUnmatched: true}},
		{3, true, Decision{Emit: true, Stop: true}},
	})
}

func TestSelectorFirstGame(t *testing.T) {
	runSteps(t, "firstgame", SelectorOptions{FirstGame: 3, StopAfter: 1}, []step{
		{1, true, Decision{ToUnmatched: true}},
		{2, true, Decision{ToUnmatched: true}},
		{3, false, Decision{ToUnmatched: true}},
		{4, true, Decision{Emit: true, Stop: true}},
	})
}

func TestSelectorGameLimit(t *testing.T) {
	runSteps(t, "gamelimit", SelectorOptions{GameLimit: 2}, []step{
		{1, false, Decision{ToUnmatched: true}},
		{2, true, Decision{Emit: true, Stop: true}},
	})
}

func TestSelectorMatchedCount(t *testing.T) {
	s := NewSelector(SelectorOptions{FirstGame: 2})
	s.Decide(1, true)
	s.Decide(2, true)
	s.Decide(3, false)
	if s.Matched() != 1 {
		t.Errorf("Matched = %d", s.Matched())
	}
}
