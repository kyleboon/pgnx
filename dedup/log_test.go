package dedup

import (
	"os"
	"testing"
)

// TestLogFuzzyDepthOracle pins Visit to the C oracle's test-fuzzydepth
// target (cmd/pgnx/testdata/golden/outfiles/test-fuzzydepthN-dupes.pgn):
// over the four games of testdata/fuzzydepth.pgn, the games the C tool
// reports as duplicates at each depth, by 1-based game number.
func TestLogFuzzyDepthOracle(t *testing.T) {
	src, err := os.ReadFile("testdata/fuzzydepth.pgn")
	if err != nil {
		t.Fatal(err)
	}
	rs := games(t, string(src))
	if len(rs) != 4 {
		t.Fatalf("%d games, want 4", len(rs))
	}
	cases := []struct {
		depth int
		dupes []int
	}{
		{0, []int{4}},
		{3, []int{3}},
		{4, []int{2, 3}},
		{5, []int{4}},
	}
	for _, c := range cases {
		log := NewLog(Options{Fuzzy: true, FuzzyDepth: c.depth})
		var got []int
		for i, r := range rs {
			if origin, dup := log.Visit(r, "fuzzydepth.pgn"); dup {
				got = append(got, i+1)
				if origin != "fuzzydepth.pgn" {
					t.Errorf("depth %d game %d: origin %q", c.depth, i+1, origin)
				}
			}
		}
		if !equalInts(got, c.dupes) {
			t.Errorf("depth %d: duplicates %v, want %v", c.depth, got, c.dupes)
		}
		if want := 4 - len(c.dupes); log.Len() != want {
			t.Errorf("depth %d: Len %d, want %d", c.depth, log.Len(), want)
		}
	}
}

func TestLogExact(t *testing.T) {
	log := NewLog(Options{})
	if _, dup := log.Visit(replayed(t, ruyLopez), "a.pgn"); dup {
		t.Error("first game reported as a duplicate")
	}
	if origin, dup := log.Visit(replayed(t, ruyLopez), "b.pgn"); !dup || origin != "a.pgn" {
		t.Errorf("identical game: dup %v origin %q, want true a.pgn", dup, origin)
	}
	if _, dup := log.Visit(replayed(t, ruyTransposed), "b.pgn"); dup {
		t.Error("a transposition is not an exact duplicate")
	}
	if log.Len() != 2 {
		t.Errorf("Len %d, want 2", log.Len())
	}
}

func TestLogOriginIsTheFirstSource(t *testing.T) {
	log := NewLog(Options{Fuzzy: true, FuzzyDepth: 0})
	log.Visit(replayed(t, ruyLopez), "first.pgn")
	log.Visit(replayed(t, foolsMate), "second.pgn")
	// A duplicate of the second game names the second source, of the
	// first (by transposition, under depth 0) the first.
	if origin, dup := log.Visit(replayed(t, foolsMate), "third.pgn"); !dup || origin != "second.pgn" {
		t.Errorf("fools mate again: %v %q", dup, origin)
	}
	if origin, dup := log.Visit(replayed(t, ruyTransposed), "third.pgn"); !dup || origin != "first.pgn" {
		t.Errorf("transposition under depth 0: %v %q", dup, origin)
	}
}

func TestLogShortGameUnderFuzzyDepthUsesExactKey(t *testing.T) {
	// A game without ply N is stored as its exact pair (hashing.c
	// previous_occurance: plycount < fuzzy_match_depth), so an identical
	// short game is a duplicate and a different one is not, and neither
	// is confused with a long game sharing a position.
	log := NewLog(Options{Fuzzy: true, FuzzyDepth: 4})
	short := "1. e4 e5 2. Nf3 *\n"
	if _, dup := log.Visit(replayed(t, short), "a"); dup {
		t.Error("first short game is a duplicate")
	}
	if _, dup := log.Visit(replayed(t, short), "a"); !dup {
		t.Error("identical short game is not a duplicate")
	}
	if _, dup := log.Visit(replayed(t, "1. e4 e5 2. Nc3 *\n"), "a"); dup {
		t.Error("different short game is a duplicate")
	}
	// The FEN game starts beyond ply 4: no fuzzy key, exact identity.
	if _, dup := log.Visit(replayed(t, fenGame), "a"); dup {
		t.Error("FEN game is a duplicate")
	}
	if _, dup := log.Visit(replayed(t, fenGame), "a"); !dup {
		t.Error("identical FEN game is not a duplicate")
	}
}

func equalInts(a, b []int) bool {
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
