package chess

// Slider attacks by magic bitboards (ADR 0005). For a rook or bishop on
// a square, the squares whose occupancy can change its attacks (the
// relevant mask: its rays without their last square) are hashed to a
// table index by one multiply and shift with a per-square constant that
// maps every distinct attack set to its own slot. The constants in
// magic_table.go were found once by internal/genmagic and are verified
// by TestSliderAttacksEverySubset; the attack tables they index are
// filled at package init (about 108,000 entries, under a millisecond).

//go:generate go run ./internal/genmagic -o magic_table.go

// magic is one square's slider hash: attacks(occ) is
// table[offset + ((occ & mask) * multiplier) >> shift].
type magic struct {
	mask       uint64
	multiplier uint64
	offset     uint32
	shift      uint8
}

var (
	rookMagics   [64]magic
	bishopMagics [64]magic

	// The attack tables, one slot per (square, relevant occupancy): the
	// sum over squares of 2 to the power of the mask's size.
	rookAttackTable   [rookTableSize]uint64
	bishopAttackTable [bishopTableSize]uint64
)

const (
	rookTableSize   = 102400
	bishopTableSize = 5248
)

// rookAttacks is the mask of squares a rook on s attacks with occ
// occupied.
func rookAttacks(s int, occ uint64) uint64 {
	m := &rookMagics[s]
	return rookAttackTable[m.offset+uint32(((occ&m.mask)*m.multiplier)>>m.shift)]
}

// bishopAttacks is the mask of squares a bishop on s attacks with occ
// occupied.
func bishopAttacks(s int, occ uint64) uint64 {
	m := &bishopMagics[s]
	return bishopAttackTable[m.offset+uint32(((occ&m.mask)*m.multiplier)>>m.shift)]
}

// queenAttacks is the mask of squares a queen on s attacks with occ
// occupied.
func queenAttacks(s int, occ uint64) uint64 {
	return rookAttacks(s, occ) | bishopAttacks(s, occ)
}

// relevantMask is the relevant-occupancy mask of a slider on s moving
// along dirs: its rays on an empty board, less each ray's last square,
// whose occupancy cannot change the attack set.
func relevantMask(s int, dirs [][2]int) uint64 {
	edges := (fileA|fileH)&^fileMask(s&7) | (rank1|rank8)&^rankMask(s>>3)
	return slideAttacks(s, dirs, 0) &^ edges
}

func init() {
	fillMagics(rookMagics[:], rookMagicNumbers[:], rookAttackTable[:], rookDirs)
	fillMagics(bishopMagics[:], bishopMagicNumbers[:], bishopAttackTable[:], bishopDirs)
}

// fillMagics computes each square's magic from its multiplier and fills
// its slots of table with the attack set of every subset of its mask.
func fillMagics(magics []magic, multipliers []uint64, table []uint64, dirs [][2]int) {
	offset := uint32(0)
	for s := 0; s < 64; s++ {
		mask := relevantMask(s, dirs)
		bitsUsed := popcount(mask)
		m := magic{mask: mask, multiplier: multipliers[s], offset: offset, shift: uint8(64 - bitsUsed)}
		magics[s] = m
		// Carry-Rippler enumeration of the subsets of mask.
		occ := uint64(0)
		for {
			table[m.offset+uint32(((occ&m.mask)*m.multiplier)>>m.shift)] = slideAttacks(s, dirs, occ)
			occ = (occ - mask) & mask
			if occ == 0 {
				break
			}
		}
		offset += 1 << uint(bitsUsed)
	}
	if int(offset) != len(table) {
		panic("chess: magic attack table size mismatch")
	}
}
