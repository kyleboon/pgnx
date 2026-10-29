package chess

// IsCheckmate reports whether the side to move is in checkmate.
func (b *Board) IsCheckmate() bool { return b.InCheck(b.SideToMove) && len(b.LegalMoves()) == 0 }

// IsStalemate reports whether the side to move is in stalemate.
func (b *Board) IsStalemate() bool { return !b.InCheck(b.SideToMove) && len(b.LegalMoves()) == 0 }

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
	var counts [3][7]int
	var bishops []Square
	for f := 0; f < 8; f++ {
		for r := 0; r < 8; r++ {
			p := b.squares[f][r]
			if p.Type == NoPiece {
				continue
			}
			counts[p.Color][p.Type]++
			if p.Type == Bishop {
				bishops = append(bishops, Square{File: f, Rank: r})
			}
		}
	}
	w, bl := counts[White], counts[Black]
	if w[Pawn] != 0 || bl[Pawn] != 0 || w[Rook] != 0 || bl[Rook] != 0 || w[Queen] != 0 || bl[Queen] != 0 {
		return false
	}
	if w[Bishop] > 1 || bl[Bishop] > 1 {
		return false
	}
	if (w[Bishop] != 0 && w[Knight] != 0) || (bl[Bishop] != 0 && bl[Knight] != 0) {
		return false
	}
	if (w[Knight] >= 2 && (bl[Bishop] != 0 || bl[Knight] != 0)) ||
		(bl[Knight] >= 2 && (w[Bishop] != 0 || w[Knight] != 0)) {
		return false
	}
	if w[Bishop] == 1 && bl[Bishop] == 1 {
		// Opposite-colour bishops: odd Manhattan parity between them.
		d := abs(bishops[0].File-bishops[1].File) + abs(bishops[0].Rank-bishops[1].Rank)
		if d%2 == 1 {
			return false
		}
	}
	return true
}
