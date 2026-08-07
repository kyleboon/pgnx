package chess

// Apply mutates the board by making move m. The caller must ensure m is
// pseudo-legal for the side to move (e.g. a move produced by LegalMoves or
// replayed from a game record); Apply does not validate legality.
func (b *Board) Apply(m Move) {
	mover := b.PieceAt(m.From)
	color := b.SideToMove

	// Step 1: en passant capture removes the pawn captured "in passing",
	// which sits beside the destination square rather than on it.
	if mover.Type == Pawn && b.HasEP && m.To == b.EPSquare && b.PieceAt(m.To).Type == NoPiece {
		b.setPiece(Square{File: m.To.File, Rank: m.From.Rank}, Piece{})
	}

	// Step 2: capture (if any) must be read before the board is mutated;
	// castling never captures.
	var captured Piece
	if m.Castle == NoCastle {
		captured = b.PieceAt(m.To)
	}

	if m.Castle != NoCastle {
		// Step 3: castling. Compute rook start file from castling rights,
		// and the king/rook destination files, then clear both the king
		// and rook squares before placing either piece (their start/end
		// squares can coincide in Chess960).
		var rookFromFile, kingToFile, rookToFile int
		if color == White {
			if m.Castle == Kingside {
				rookFromFile = b.Castling.WhiteKRookFile
			} else {
				rookFromFile = b.Castling.WhiteQRookFile
			}
		} else {
			if m.Castle == Kingside {
				rookFromFile = b.Castling.BlackKRookFile
			} else {
				rookFromFile = b.Castling.BlackQRookFile
			}
		}
		if m.Castle == Kingside {
			kingToFile, rookToFile = 6, 5
		} else {
			kingToFile, rookToFile = 2, 3
		}

		rank := m.From.Rank
		rookFrom := Square{File: rookFromFile, Rank: rank}
		kingTo := Square{File: kingToFile, Rank: rank}
		rookTo := Square{File: rookToFile, Rank: rank}

		rook := Piece{Type: Rook, Color: color}
		king := Piece{Type: King, Color: color}

		b.setPiece(m.From, Piece{})
		b.setPiece(rookFrom, Piece{})
		b.setPiece(kingTo, king)
		b.setPiece(rookTo, rook)
	} else {
		// Step 4: normal move (including captures and promotions): clear
		// the origin square and place the moving piece (or promoted piece)
		// on the destination square.
		b.setPiece(m.From, Piece{})
		placed := mover
		if m.Promotion != NoPiece {
			placed = Piece{Type: m.Promotion, Color: color}
		}
		b.setPiece(m.To, placed)
	}

	// Step 5: castling-rights updates.
	if mover.Type == King || m.Castle != NoCastle {
		if color == White {
			b.Castling.WhiteKingside = false
			b.Castling.WhiteQueenside = false
		} else {
			b.Castling.BlackKingside = false
			b.Castling.BlackQueenside = false
		}
	}
	if mover.Type == Rook && m.Castle == NoCastle {
		clearRookRight(b, color, m.From)
	}
	if m.Castle == NoCastle && captured.Type == Rook {
		clearRookRight(b, color.Opponent(), m.To)
	}

	// Step 6: en passant state is only set for a two-square pawn push.
	if mover.Type == Pawn && abs(m.To.Rank-m.From.Rank) == 2 {
		b.HasEP = true
		b.EPSquare = Square{File: m.From.File, Rank: (m.From.Rank + m.To.Rank) / 2}
	} else {
		b.HasEP = false
		b.EPSquare = Square{}
	}

	// Step 7: clocks and side to move.
	if mover.Type == Pawn || captured.Type != NoPiece {
		b.HalfmoveClock = 0
	} else {
		b.HalfmoveClock++
	}
	if color == Black {
		b.FullmoveNumber++
	}
	b.SideToMove = color.Opponent()
}

// clearRookRight clears owner's castling right whose rook-start square is
// sq, if that right is currently set and sq is on owner's back rank.
func clearRookRight(b *Board, owner Color, sq Square) {
	backRank := 0
	if owner == Black {
		backRank = 7
	}
	if sq.Rank != backRank {
		return
	}
	if owner == White {
		if b.Castling.WhiteKingside && sq.File == b.Castling.WhiteKRookFile {
			b.Castling.WhiteKingside = false
		}
		if b.Castling.WhiteQueenside && sq.File == b.Castling.WhiteQRookFile {
			b.Castling.WhiteQueenside = false
		}
	} else {
		if b.Castling.BlackKingside && sq.File == b.Castling.BlackKRookFile {
			b.Castling.BlackKingside = false
		}
		if b.Castling.BlackQueenside && sq.File == b.Castling.BlackQRookFile {
			b.Castling.BlackQueenside = false
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
