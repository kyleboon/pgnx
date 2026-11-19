package chess

import "math/bits"

// The board's private representation is a set of 64-bit masks, one bit
// per square (ADR 0005). Bit 0 is a1, bit 7 h1, bit 8 a2 and bit 63 h8:
// the square's index is 8*rank+file, the ordering the Polyglot hash also
// uses. Square, the exported coordinate pair, converts to and from that
// index at the method boundary.

// sqIndex is the bit index of sq.
func sqIndex(sq Square) int { return sq.Rank<<3 | sq.File }

// sqOf is the Square of bit index i.
func sqOf(i int) Square { return Square{File: i & 7, Rank: i >> 3} }

// bit is the mask holding only bit i.
func bit(i int) uint64 { return 1 << uint(i) }

// Rank and file masks.
const (
	fileA uint64 = 0x0101010101010101
	fileH uint64 = fileA << 7
	rank1 uint64 = 0xFF
	rank3 uint64 = rank1 << 16
	rank6 uint64 = rank1 << 40
	rank8 uint64 = rank1 << 56
	// lightSquares holds the squares a light-squared bishop moves on
	// (h1 is light).
	lightSquares uint64 = 0x55AA55AA55AA55AA
)

// rankMask is the mask of rank r (0 for the first rank).
func rankMask(r int) uint64 { return rank1 << (8 * uint(r)) }

// fileMask is the mask of file f (0 for the a-file).
func fileMask(f int) uint64 { return fileA << uint(f) }

// Leaper attack tables, filled at init: the squares a knight, king or
// pawn of the given colour on the indexed square attacks.
var (
	knightAttacks [64]uint64
	kingAttacks   [64]uint64
	pawnAttacks   [3][64]uint64 // indexed by Color; NoColor unused
)

// stepAttacks is the mask of the squares reached from s by one step of
// each of steps, those that stay on the board.
func stepAttacks(s int, steps [][2]int) uint64 {
	var m uint64
	file, rank := s&7, s>>3
	for _, d := range steps {
		f, r := file+d[0], rank+d[1]
		if f >= 0 && f < 8 && r >= 0 && r < 8 {
			m |= bit(r<<3 | f)
		}
	}
	return m
}

// rays is the mask of the squares from the indexed square to the edge of
// the board in each compass direction, exclusive of the square itself,
// indexed by dirIndex. A package variable rather than an init product so
// the magic tables' init can rely on it.
var rays = buildRays()

// dirIndex numbers a step (df, dr) 0 to 8, with the positive directions
// (a greater rank, or the same rank and a greater file) above 4, the
// index of the null step.
func dirIndex(d [2]int) int { return (d[1]+1)*3 + d[0] + 1 }

func buildRays() (r [9][64]uint64) {
	for _, d := range append(append([][2]int{}, rookDirs...), bishopDirs...) {
		for s := 0; s < 64; s++ {
			file, rank := s&7, s>>3
			for f, k := file+d[0], rank+d[1]; f >= 0 && f < 8 && k >= 0 && k < 8; f, k = f+d[0], k+d[1] {
				r[dirIndex(d)][s] |= bit(k<<3 | f)
			}
		}
	}
	return r
}

// slideAttacks is the mask of the squares a slider on s reaches along
// each of dirs with occ occupied: every empty square and the first
// occupied one, which may be either colour's piece. Along a ray the
// first blocker is the nearest set bit of the ray's occupancy, the
// lowest in a positive direction and the highest in a negative one, and
// the squares beyond it are that blocker's own ray.
func slideAttacks(s int, dirs [][2]int, occ uint64) uint64 {
	var m uint64
	for _, d := range dirs {
		i := dirIndex(d)
		ray := rays[i][s]
		if blockers := ray & occ; blockers != 0 {
			first := lowest(blockers)
			if i < 4 {
				first = 63 - bits.LeadingZeros64(blockers)
			}
			ray &^= rays[i][first]
		}
		m |= ray
	}
	return m
}

func init() {
	for s := 0; s < 64; s++ {
		knightAttacks[s] = stepAttacks(s, knightDeltas)
		kingAttacks[s] = stepAttacks(s, kingDeltas)
		pawnAttacks[White][s] = stepAttacks(s, [][2]int{{-1, 1}, {1, 1}})
		pawnAttacks[Black][s] = stepAttacks(s, [][2]int{{-1, -1}, {1, -1}})
	}
}

// popcount is the number of squares in m.
func popcount(m uint64) int { return bits.OnesCount64(m) }

// lowest is the index of the lowest set bit of m, which must not be 0.
func lowest(m uint64) int { return bits.TrailingZeros64(m) }
