package chess

var (
	knightDeltas = [][2]int{{1, 2}, {2, 1}, {2, -1}, {1, -2}, {-1, -2}, {-2, -1}, {-2, 1}, {-1, 2}}
	kingDeltas   = [][2]int{{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}}
	rookDirs     = [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	bishopDirs   = [][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
)

func (b *Board) IsAttacked(sq Square, by Color) bool {
	for _, d := range knightDeltas {
		if b.pieceIs(Square{sq.File + d[0], sq.Rank + d[1]}, Knight, by) {
			return true
		}
	}
	for _, d := range kingDeltas {
		if b.pieceIs(Square{sq.File + d[0], sq.Rank + d[1]}, King, by) {
			return true
		}
	}
	// Pawns: a white pawn on rank r attacks r+1, so look one rank *toward* the attacker.
	pr := -1
	if by == White {
		pr = 1
	}
	for _, df := range []int{-1, 1} {
		if b.pieceIs(Square{sq.File + df, sq.Rank - pr}, Pawn, by) {
			return true
		}
	}
	if b.rayHits(sq, rookDirs, by, Rook) || b.rayHits(sq, bishopDirs, by, Bishop) {
		return true
	}
	return false
}

func (b *Board) rayHits(sq Square, dirs [][2]int, by Color, slider PieceType) bool {
	for _, d := range dirs {
		cur := Square{sq.File + d[0], sq.Rank + d[1]}
		for cur.Valid() {
			p := b.PieceAt(cur)
			if p.Type != NoPiece {
				if p.Color == by && (p.Type == slider || p.Type == Queen) {
					return true
				}
				break
			}
			cur = Square{cur.File + d[0], cur.Rank + d[1]}
		}
	}
	return false
}

func (b *Board) pieceIs(sq Square, pt PieceType, c Color) bool {
	return sq.Valid() && b.PieceAt(sq) == Piece{pt, c}
}

func (b *Board) InCheck(c Color) bool {
	return b.IsAttacked(b.KingSquare(c), c.Opponent())
}
