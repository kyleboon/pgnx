package chess

import (
	"math/bits"
	"math/rand"
	"testing"
)

// refAttacks is the reference for the attack tables, written from the
// geometry of a square's coordinates rather than from the package's
// masks: the squares reachable from s by steps of (df, dr), one step
// for a leaper and until a blocker in occ (inclusive) for a slider.
func refAttacks(s int, steps [][2]int, slider bool, occ uint64) uint64 {
	var attacks uint64
	file, rank := s%8, s/8
	for _, d := range steps {
		f, r := file+d[0], rank+d[1]
		for f >= 0 && f < 8 && r >= 0 && r < 8 {
			to := r*8 + f
			attacks |= 1 << to
			if !slider || occ&(1<<to) != 0 {
				break
			}
			f, r = f+d[0], r+d[1]
		}
	}
	return attacks
}

var (
	refKnightSteps = [][2]int{{1, 2}, {2, 1}, {2, -1}, {1, -2}, {-1, -2}, {-2, -1}, {-2, 1}, {-1, 2}}
	refKingSteps   = [][2]int{{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}}
	refRookSteps   = [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	refBishopSteps = [][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
)

func TestLeaperAttackTables(t *testing.T) {
	for s := 0; s < 64; s++ {
		if got, want := knightAttacks[s], refAttacks(s, refKnightSteps, false, 0); got != want {
			t.Errorf("knightAttacks[%v] = %#x, want %#x", sqOf(s), got, want)
		}
		if got, want := kingAttacks[s], refAttacks(s, refKingSteps, false, 0); got != want {
			t.Errorf("kingAttacks[%v] = %#x, want %#x", sqOf(s), got, want)
		}
		// A white pawn attacks the two squares diagonally ahead of it.
		if got, want := pawnAttacks[White][s], refAttacks(s, [][2]int{{-1, 1}, {1, 1}}, false, 0); got != want {
			t.Errorf("pawnAttacks[White][%v] = %#x, want %#x", sqOf(s), got, want)
		}
		if got, want := pawnAttacks[Black][s], refAttacks(s, [][2]int{{-1, -1}, {1, -1}}, false, 0); got != want {
			t.Errorf("pawnAttacks[Black][%v] = %#x, want %#x", sqOf(s), got, want)
		}
	}
}

// TestSliderAttacksEverySubset checks the magic lookup on every subset
// of every square's relevant-occupancy mask, so each table entry is
// visited once, and then on random full occupancies, which exercise the
// masking of the irrelevant squares.
func TestSliderAttacksEverySubset(t *testing.T) {
	for s := 0; s < 64; s++ {
		for _, c := range []struct {
			name  string
			steps [][2]int
			mask  uint64
			look  func(int, uint64) uint64
		}{
			{"rook", refRookSteps, rookMagics[s].mask, rookAttacks},
			{"bishop", refBishopSteps, bishopMagics[s].mask, bishopAttacks},
		} {
			if want := refAttacks(s, c.steps, true, 0) &^ edgesAround(s); c.mask != want {
				t.Errorf("%s mask[%v] = %#x, want %#x", c.name, sqOf(s), c.mask, want)
			}
			n := 0
			for occ := uint64(0); ; { // Carry-Rippler subset walk
				if got, want := c.look(s, occ), refAttacks(s, c.steps, true, occ); got != want {
					t.Fatalf("%sAttacks(%v, %#x) = %#x, want %#x", c.name, sqOf(s), occ, got, want)
				}
				n++
				if occ = (occ - c.mask) & c.mask; occ == 0 {
					break
				}
			}
			if n != 1<<bits.OnesCount64(c.mask) {
				t.Errorf("%s %v: visited %d subsets, want %d", c.name, sqOf(s), n, 1<<bits.OnesCount64(c.mask))
			}
		}
	}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 20000; i++ {
		occ := rng.Uint64() & rng.Uint64()
		s := rng.Intn(64)
		if got, want := rookAttacks(s, occ), refAttacks(s, refRookSteps, true, occ); got != want {
			t.Fatalf("rookAttacks(%v, %#x) = %#x, want %#x", sqOf(s), occ, got, want)
		}
		if got, want := bishopAttacks(s, occ), refAttacks(s, refBishopSteps, true, occ); got != want {
			t.Fatalf("bishopAttacks(%v, %#x) = %#x, want %#x", sqOf(s), occ, got, want)
		}
	}
}

// edgesAround is the board edge squares that a relevant-occupancy mask
// for s leaves out: a blocker on the last square of a ray changes
// nothing, so the magic index ignores the edges, except the edges of
// s's own rank or file, which the mask keeps for the perpendicular rays.
func edgesAround(s int) uint64 {
	const fileA, fileH, rank1, rank8 uint64 = 0x0101010101010101, 0x8080808080808080, 0xFF, 0xFF << 56
	edges := (fileA|fileH)&^fileOf(s) | (rank1|rank8)&^rankOf(s)
	return edges
}

func fileOf(s int) uint64 { return 0x0101010101010101 << (s % 8) }
func rankOf(s int) uint64 { return 0xFF << (8 * (s / 8)) }

// TestSquareIndexRoundTrip pins the square numbering the bitboards use:
// a1 is bit 0, h1 bit 7, a8 bit 56, which is also the Polyglot ordering
// the placement key indexes by.
func TestSquareIndexRoundTrip(t *testing.T) {
	for s := 0; s < 64; s++ {
		if got := sqIndex(sqOf(s)); got != s {
			t.Errorf("sqIndex(sqOf(%d)) = %d", s, got)
		}
	}
	for _, c := range []struct {
		name string
		idx  int
	}{{"a1", 0}, {"h1", 7}, {"e4", 28}, {"a8", 56}, {"h8", 63}} {
		if got := sqIndex(MustSquare(c.name)); got != c.idx {
			t.Errorf("sqIndex(%s) = %d, want %d", c.name, got, c.idx)
		}
	}
}
