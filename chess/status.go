package chess

// IsCheckmate reports whether the side to move is in checkmate.
func (b *Board) IsCheckmate() bool { return b.InCheck(b.SideToMove) && !b.HasLegalMove() }

// IsStalemate reports whether the side to move is in stalemate.
func (b *Board) IsStalemate() bool { return !b.InCheck(b.SideToMove) && !b.HasLegalMove() }

// InsufficientMaterial reports whether neither side has enough material
// to force checkmate, using pgn-extract's definition (end.c
// insufficient_material + opposite_colour_bishops) rather than the FIDE
// one. The C rule is deliberately loose: no pawns, rooks, or queens on
// either side; at most one bishop per side; no side with both a bishop
// and a knight; a side with two or more knights fails only if the
// opponent has any minor piece (so KNN v K counts as insufficient); and
// a lone bishop each fails only when they stand on opposite-colour
// squares. It is ported as-is so --insufficient and --fix-result-tags
// agree with the oracle.
func (b *Board) InsufficientMaterial() bool {
	if b.byType[Pawn]|b.byType[Rook]|b.byType[Queen] != 0 {
		return false
	}
	wB, bB := b.pieces(Bishop, White), b.pieces(Bishop, Black)
	wN, bN := b.pieces(Knight, White), b.pieces(Knight, Black)
	if popcount(wB) > 1 || popcount(bB) > 1 {
		return false
	}
	if (wB != 0 && wN != 0) || (bB != 0 && bN != 0) {
		return false
	}
	if (popcount(wN) >= 2 && bB|bN != 0) || (popcount(bN) >= 2 && wB|wN != 0) {
		return false
	}
	if wB != 0 && bB != 0 && (wB&lightSquares != 0) != (bB&lightSquares != 0) {
		// Opposite-colour bishops.
		return false
	}
	return true
}
