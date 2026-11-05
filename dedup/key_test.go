package dedup

import (
	"testing"

	"github.com/kyleboon/pgnx/chess"
)

func TestKeyOfIdentity(t *testing.T) {
	a, b := KeyOf(replayed(t, foolsMate)), KeyOf(replayed(t, foolsMate))
	if a != b {
		t.Errorf("identical games: %+v vs %+v", a, b)
	}
	if a == KeyOf(replayed(t, ruyLopez)) {
		t.Error("different games share a key")
	}
}

func TestKeyOfTranspositionSharesFinalOnly(t *testing.T) {
	a, b := KeyOf(replayed(t, ruyLopez)), KeyOf(replayed(t, ruyTransposed))
	if a.Final != b.Final {
		t.Errorf("transposition: Final %x vs %x", a.Final, b.Final)
	}
	if a.Cumulative == b.Cumulative {
		t.Error("transposition: Cumulative keys equal, want different")
	}
}

func TestKeyOfFinalIsTheFinalBoard(t *testing.T) {
	r := replayed(t, foolsMate)
	if k := KeyOf(r); k.Final != r.FinalBoard().PlacementKey() {
		t.Errorf("Final %x, want the final board's placement %x", k.Final, r.FinalBoard().PlacementKey())
	}
}

func TestKeyOfNoMoves(t *testing.T) {
	k := KeyOf(replayed(t, "*\n"))
	if k.Final != chess.NewBoard().PlacementKey() {
		t.Errorf("Final %x, want the start placement", k.Final)
	}
	if k.Cumulative != 0 {
		t.Errorf("Cumulative %x, want 0", k.Cumulative)
	}
	if k := KeyOf(nil); k != (Key{}) {
		t.Errorf("KeyOf(nil) = %+v, want zero", k)
	}
}

func TestFuzzyKey(t *testing.T) {
	r := replayed(t, ruyLopez)
	afterThree := replayed(t, "1. e4 e5 2. Nf3 *\n").FinalBoard().PlacementKey()
	cases := []struct {
		name  string
		depth int
		want  uint64
		ok    bool
	}{
		{"depth 0 is the final position", 0, KeyOf(r).Final, true},
		{"depth 3 is the position after ply 3", 3, afterThree, true},
		{"depth 5 is the final position of a 5-ply game", 5, KeyOf(r).Final, true},
		{"depth 6 is beyond a 5-ply game", 6, 0, false},
	}
	for _, c := range cases {
		got, ok := FuzzyKey(r, c.depth)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("%s: FuzzyKey = %x, %v; want %x, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestFuzzyKeyCountsAbsolutePlies(t *testing.T) {
	// The FEN game's first move is ply 41 (apply.c play_moves: plies
	// starts at 2*move_number-1), so it has no ply 3 and ply 42 is its
	// second move.
	r := replayed(t, fenGame)
	if _, ok := FuzzyKey(r, 3); ok {
		t.Error("depth 3 on a game starting at ply 41: want no key")
	}
	afterTwo := replayed(t, "[SetUp \"1\"]\n[FEN \"r1bq1rk1/pp3ppp/2n2n2/3pp3/1b1P4/2NBPN2/PP3PPP/R2Q1RK1 w - - 0 21\"]\n\n21. dxe5 Nxe5 *\n").FinalBoard().PlacementKey()
	if got, ok := FuzzyKey(r, 42); !ok || got != afterTwo {
		t.Errorf("depth 42: %x, %v; want %x, true", got, ok, afterTwo)
	}
	if _, ok := FuzzyKey(nil, 0); ok {
		t.Error("FuzzyKey(nil): want no key")
	}
}
