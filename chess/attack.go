package chess

// Piece step patterns, used to fill the leaper attack tables and the
// slider masks at init.
var (
	knightDeltas = [][2]int{{1, 2}, {2, 1}, {2, -1}, {1, -2}, {-1, -2}, {-2, -1}, {-2, 1}, {-1, 2}}
	kingDeltas   = [][2]int{{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}}
	rookDirs     = [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	bishopDirs   = [][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
)

// IsAttacked reports whether sq is attacked by any piece of color by,
// considering knight, king, pawn, and sliding (rook/bishop/queen) attacks.
// It does not consider whose turn it is to move.
func (b *Board) IsAttacked(sq Square, by Color) bool {
	return b.attacked(sqIndex(sq), by)
}

// attacked is IsAttacked on a bit index: each piece kind's attackers of
// s are the pieces of that kind on the squares a like piece on s would
// attack (for a pawn, one of the other colour), a table lookup per kind
// and a magic lookup per slider line.
func (b *Board) attacked(s int, by Color) bool {
	them := b.byColor[by]
	leapers := knightAttacks[s]&b.byType[Knight] |
		kingAttacks[s]&b.byType[King] |
		pawnAttacks[by.Opponent()][s]&b.byType[Pawn]
	if leapers&them != 0 {
		return true
	}
	occ := b.occupied()
	if rookAttacks(s, occ)&(b.byType[Rook]|b.byType[Queen])&them != 0 {
		return true
	}
	return bishopAttacks(s, occ)&(b.byType[Bishop]|b.byType[Queen])&them != 0
}

// InCheck reports whether color c's king is currently attacked.
func (b *Board) InCheck(c Color) bool {
	return b.attacked(b.kingIndex(c), c.Opponent())
}
