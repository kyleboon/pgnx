package chess

// IsCheckmate reports whether the side to move is in checkmate.
func (b *Board) IsCheckmate() bool { return b.InCheck(b.SideToMove) && len(b.LegalMoves()) == 0 }

// IsStalemate reports whether the side to move is in stalemate.
func (b *Board) IsStalemate() bool { return !b.InCheck(b.SideToMove) && len(b.LegalMoves()) == 0 }
