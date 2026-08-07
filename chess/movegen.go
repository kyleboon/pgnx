package chess

// promotionPieces lists the piece types a pawn may promote to, in the order
// promotion moves are generated.
var promotionPieces = []PieceType{Queen, Rook, Bishop, Knight}

// LegalMoves returns all fully legal moves for the side to move: pseudo-legal
// moves that do not leave the mover's own king in check.
func (b *Board) LegalMoves() []Move {
	mover := b.SideToMove
	pseudo := b.pseudoLegalMoves()
	moves := make([]Move, 0, len(pseudo))
	for _, m := range pseudo {
		c := b.Copy()
		c.Apply(m)
		if !c.InCheck(mover) {
			moves = append(moves, m)
		}
	}
	return moves
}

// pseudoLegalMoves returns all pseudo-legal moves for the side to move:
// moves that follow piece movement rules but may leave the mover's own king
// in check.
func (b *Board) pseudoLegalMoves() []Move {
	var moves []Move
	color := b.SideToMove
	for file := 0; file < 8; file++ {
		for rank := 0; rank < 8; rank++ {
			sq := Square{File: file, Rank: rank}
			p := b.PieceAt(sq)
			if p.Type == NoPiece || p.Color != color {
				continue
			}
			switch p.Type {
			case Pawn:
				b.genPawnMoves(sq, color, &moves)
			case Knight:
				b.genDeltaMoves(sq, color, knightDeltas, &moves)
			case King:
				b.genDeltaMoves(sq, color, kingDeltas, &moves)
			case Rook:
				b.genSliderMoves(sq, color, rookDirs, &moves)
			case Bishop:
				b.genSliderMoves(sq, color, bishopDirs, &moves)
			case Queen:
				b.genSliderMoves(sq, color, rookDirs, &moves)
				b.genSliderMoves(sq, color, bishopDirs, &moves)
			}
		}
	}
	b.genCastleMoves(color, &moves)
	return moves
}

// genDeltaMoves appends moves for a piece that moves by fixed deltas
// (knight, king): the target must be on-board and not occupied by a piece of
// the same color.
func (b *Board) genDeltaMoves(from Square, color Color, deltas [][2]int, moves *[]Move) {
	for _, d := range deltas {
		to := Square{File: from.File + d[0], Rank: from.Rank + d[1]}
		if !to.Valid() {
			continue
		}
		target := b.PieceAt(to)
		if target.Type != NoPiece && target.Color == color {
			continue
		}
		*moves = append(*moves, Move{From: from, To: to})
	}
}

// genSliderMoves appends moves for a sliding piece (rook/bishop/queen):
// walk each direction until the edge of the board, a friendly piece
// (excluded), or an enemy piece (included, ray stops there).
func (b *Board) genSliderMoves(from Square, color Color, dirs [][2]int, moves *[]Move) {
	for _, d := range dirs {
		to := Square{File: from.File + d[0], Rank: from.Rank + d[1]}
		for to.Valid() {
			target := b.PieceAt(to)
			if target.Type == NoPiece {
				*moves = append(*moves, Move{From: from, To: to})
				to = Square{File: to.File + d[0], Rank: to.Rank + d[1]}
				continue
			}
			if target.Color != color {
				*moves = append(*moves, Move{From: from, To: to})
			}
			break
		}
	}
}

// genPawnMoves appends single/double pushes, diagonal captures, en passant
// captures, and promotion expansions for the pawn on sq.
func (b *Board) genPawnMoves(sq Square, color Color, moves *[]Move) {
	dir := 1
	startRank := 1
	lastRank := 7
	if color == Black {
		dir = -1
		startRank = 6
		lastRank = 0
	}

	// Single push.
	one := Square{File: sq.File, Rank: sq.Rank + dir}
	if one.Valid() && b.PieceAt(one).Type == NoPiece {
		b.addPawnMove(sq, one, lastRank, moves)

		// Double push, only from the starting rank and only if both the
		// intermediate and destination squares are empty.
		if sq.Rank == startRank {
			two := Square{File: sq.File, Rank: sq.Rank + 2*dir}
			if b.PieceAt(two).Type == NoPiece {
				*moves = append(*moves, Move{From: sq, To: two})
			}
		}
	}

	// Diagonal captures (including en passant).
	for _, df := range []int{-1, 1} {
		to := Square{File: sq.File + df, Rank: sq.Rank + dir}
		if !to.Valid() {
			continue
		}
		target := b.PieceAt(to)
		if target.Type != NoPiece && target.Color != color {
			b.addPawnMove(sq, to, lastRank, moves)
			continue
		}
		if b.HasEP && to == b.EPSquare && target.Type == NoPiece {
			*moves = append(*moves, Move{From: sq, To: to})
		}
	}
}

// addPawnMove appends m (or, if the destination is on the last rank, its
// four promotion expansions).
func (b *Board) addPawnMove(from, to Square, lastRank int, moves *[]Move) {
	if to.Rank == lastRank {
		for _, pt := range promotionPieces {
			*moves = append(*moves, Move{From: from, To: to, Promotion: pt})
		}
		return
	}
	*moves = append(*moves, Move{From: from, To: to})
}

// genCastleMoves appends legal castling moves for color. Generic over rook
// start files so it also works for Chess960: the squares that must be
// vacant are the union of the king's path and the rook's path, excluding
// the king and rook's own starting squares; the king must not start in
// check, and every square it passes through (inclusive of its destination)
// must not be attacked by the opponent.
func (b *Board) genCastleMoves(color Color, moves *[]Move) {
	kingSq := b.KingSquare(color)
	backRank := 0
	if color == Black {
		backRank = 7
	}
	opponent := color.Opponent()

	tryCastle := func(has bool, rookFile int, side CastleSide, kingToFile, rookToFile int) {
		if !has {
			return
		}
		rookSq := Square{File: rookFile, Rank: backRank}
		kingTo := Square{File: kingToFile, Rank: backRank}

		// All squares in the king's path and the rook's path must be empty,
		// except for the king's and rook's own starting squares.
		lo, hi := kingSq.File, kingToFile
		if lo > hi {
			lo, hi = hi, lo
		}
		for f := lo; f <= hi; f++ {
			s := Square{File: f, Rank: backRank}
			if s == kingSq || s == rookSq {
				continue
			}
			if b.PieceAt(s).Type != NoPiece {
				return
			}
		}
		lo, hi = rookFile, rookToFile
		if lo > hi {
			lo, hi = hi, lo
		}
		for f := lo; f <= hi; f++ {
			s := Square{File: f, Rank: backRank}
			if s == kingSq || s == rookSq {
				continue
			}
			if b.PieceAt(s).Type != NoPiece {
				return
			}
		}

		// The king may not be in check, nor cross (inclusive of its
		// destination) any square attacked by the opponent.
		lo, hi = kingSq.File, kingToFile
		if lo > hi {
			lo, hi = hi, lo
		}
		for f := lo; f <= hi; f++ {
			s := Square{File: f, Rank: backRank}
			if b.IsAttacked(s, opponent) {
				return
			}
		}

		*moves = append(*moves, Move{From: kingSq, To: kingTo, Castle: side})
	}

	if color == White {
		tryCastle(b.Castling.WhiteKingside, b.Castling.WhiteKRookFile, Kingside, 6, 5)
		tryCastle(b.Castling.WhiteQueenside, b.Castling.WhiteQRookFile, Queenside, 2, 3)
	} else {
		tryCastle(b.Castling.BlackKingside, b.Castling.BlackKRookFile, Kingside, 6, 5)
		tryCastle(b.Castling.BlackQueenside, b.Castling.BlackQRookFile, Queenside, 2, 3)
	}
}
