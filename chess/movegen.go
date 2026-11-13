package chess

// promotionPieces lists the piece types a pawn may promote to, in the order
// promotion moves are generated.
var promotionPieces = []PieceType{Queen, Rook, Bishop, Knight}

// LegalMoves returns all fully legal moves for the side to move: pseudo-legal
// moves that do not leave the mover's own king in check. It allocates a
// fresh slice per call; a caller generating repeatedly keeps one buffer
// and uses AppendLegalMoves instead.
func (b *Board) LegalMoves() []Move {
	return b.AppendLegalMoves(make([]Move, 0, legalMovesHint))
}

// legalMovesHint is the capacity LegalMoves allocates up front: enough for
// the pseudo-legal moves of nearly every position reached in play, so the
// slice is not regrown while generating. It is deliberately smaller than
// stackMoveBuffer: a heap slice this size is paid by every LegalMoves
// caller and a regrow there only costs a second allocation, whereas the
// stack buffer is free to size for the worst case.
const legalMovesHint = 64

// AppendLegalMoves appends the legal moves of the side to move to moves and
// returns the extended slice, in the same order as LegalMoves. Nothing is
// allocated when moves has enough spare capacity for the position's
// pseudo-legal moves, so a caller that keeps the returned slice and passes
// it back as moves[:0] generates allocation-free once it has grown.
func (b *Board) AppendLegalMoves(moves []Move) []Move {
	start := len(moves)
	return b.keepLegal(b.appendPseudoLegalMoves(moves), start)
}

// AppendLegalMovesTo appends the legal moves of the side to move by which
// a piece of type pt reaches to, and returns the extended slice: the
// candidates a SAN move naming that piece and destination can denote. It
// looks outward from to, as pgn-extract's map.c find_knight_moves,
// find_bishop_moves and find_pawn_moves do, so it never visits more than
// the squares a piece of that type could arrive from; a pawn's push,
// double push, capture and en passant capture origins are all
// considered, and a pawn reaching the last rank is expanded into its
// promotions. Castling is never among the moves, even when the king's
// castling destination is to. The candidates pass the same
// leave-king-in-check filter as AppendLegalMoves and allocate nothing
// when moves has spare capacity.
func (b *Board) AppendLegalMovesTo(pt PieceType, to Square, moves []Move) []Move {
	start := len(moves)
	color := b.SideToMove
	switch pt {
	case Pawn:
		moves = b.findPawnMoves(to, color, moves)
	case Knight:
		moves = b.findDeltaMoves(to, Knight, color, knightDeltas, moves)
	case King:
		moves = b.findDeltaMoves(to, King, color, kingDeltas, moves)
	case Rook:
		moves = b.findSliderMoves(to, Rook, color, rookDirs, moves)
	case Bishop:
		moves = b.findSliderMoves(to, Bishop, color, bishopDirs, moves)
	case Queen:
		moves = b.findSliderMoves(to, Queen, color, rookDirs, moves)
		moves = b.findSliderMoves(to, Queen, color, bishopDirs, moves)
	}
	return b.keepLegal(moves, start)
}

// keepLegal filters moves[start:] in place to the pseudo-legal moves
// whose making leaves the mover's king unattacked, compacting them
// towards start, and returns the shortened slice. The legality probe is
// a stack copy of the board, so no candidate allocates.
func (b *Board) keepLegal(moves []Move, start int) []Move {
	mover := b.SideToMove
	w := start
	for i := start; i < len(moves); i++ {
		m := moves[i]
		c := *b
		c.Apply(m)
		if !c.InCheck(mover) {
			moves[w] = m
			w++
		}
	}
	return moves[:w]
}

// findDeltaMoves appends the moves to to of color's pieces of type pt
// that move by fixed deltas (knight, king): one from each delta's origin
// square that holds such a piece, provided to is not occupied by a piece
// of the same color.
func (b *Board) findDeltaMoves(to Square, pt PieceType, color Color, deltas [][2]int, moves []Move) []Move {
	if target := b.PieceAt(to); target.Type != NoPiece && target.Color == color {
		return moves
	}
	for _, d := range deltas {
		from := Square{File: to.File - d[0], Rank: to.Rank - d[1]}
		if b.pieceIs(from, pt, color) {
			moves = append(moves, Move{From: from, To: to})
		}
	}
	return moves
}

// findSliderMoves appends the moves to to of color's sliding pieces of
// type pt: along each direction the first piece met, if it is such a
// piece, has a clear ray to to. Nothing is found when to holds a piece of
// the same color.
func (b *Board) findSliderMoves(to Square, pt PieceType, color Color, dirs [][2]int, moves []Move) []Move {
	if target := b.PieceAt(to); target.Type != NoPiece && target.Color == color {
		return moves
	}
	for _, d := range dirs {
		from := Square{File: to.File + d[0], Rank: to.Rank + d[1]}
		for from.Valid() {
			p := b.PieceAt(from)
			if p.Type == NoPiece {
				from = Square{File: from.File + d[0], Rank: from.Rank + d[1]}
				continue
			}
			if p.Type == pt && p.Color == color {
				moves = append(moves, Move{From: from, To: to})
			}
			break
		}
	}
	return moves
}

// findPawnMoves appends the moves to to of color's pawns: the single or
// double push when to is empty, the diagonal captures when to holds an
// enemy piece or is the en passant square, with the same rules and
// promotion expansion as genPawnMoves.
func (b *Board) findPawnMoves(to Square, color Color, moves []Move) []Move {
	dir, startRank, lastRank, epRank := pawnGeometry(color)

	target := b.PieceAt(to)
	if target.Type == NoPiece {
		// Pushes: the pawn one square back, or two squares back from
		// its starting rank with the square between empty.
		one := Square{File: to.File, Rank: to.Rank - dir}
		if one.Valid() {
			switch {
			case b.pieceIs(one, Pawn, color):
				moves = addPawnMove(one, to, lastRank, moves)
			case b.PieceAt(one).Type == NoPiece && to.Rank == startRank+2*dir:
				two := Square{File: to.File, Rank: startRank}
				if b.pieceIs(two, Pawn, color) {
					moves = append(moves, Move{From: two, To: to})
				}
			}
		}
		if !(b.HasEP && to == b.EPSquare && to.Rank == epRank) {
			return moves
		}
		// En passant: the capturing pawns sit diagonally behind to.
		for _, df := range [2]int{-1, 1} {
			from := Square{File: to.File + df, Rank: to.Rank - dir}
			if b.pieceIs(from, Pawn, color) {
				moves = append(moves, Move{From: from, To: to})
			}
		}
		return moves
	}
	if target.Color == color {
		return moves
	}
	for _, df := range [2]int{-1, 1} {
		from := Square{File: to.File + df, Rank: to.Rank - dir}
		if b.pieceIs(from, Pawn, color) {
			moves = addPawnMove(from, to, lastRank, moves)
		}
	}
	return moves
}

// stackMoveBuffer is the size of the move buffer HasLegalMove keeps on the
// stack. No legal position has more than 218 legal moves, and none reached
// in play has that many pseudo-legal ones, so the buffer is never regrown
// (and if an unreal position did regrow it, only an allocation would
// follow, never a wrong answer).
const stackMoveBuffer = 256

// HasLegalMove reports whether the side to move has at least one legal
// move. It probes the pseudo-legal moves in generation order and stops at
// the first legal one, allocating nothing, so a caller that already knows
// the side to move is in check decides mate without generating every
// reply (as map.c king_is_in_checkmate stops at the first saving move).
func (b *Board) HasLegalMove() bool {
	var buf [stackMoveBuffer]Move
	mover := b.SideToMove
	for _, m := range b.appendPseudoLegalMoves(buf[:0]) {
		c := *b
		c.Apply(m)
		if !c.InCheck(mover) {
			return true
		}
	}
	return false
}

// appendPseudoLegalMoves appends all pseudo-legal moves for the side to
// move: moves that follow piece movement rules but may leave the mover's
// own king in check. The helpers take and return the slice rather than a
// pointer to it so a caller's stack buffer stays on the stack.
func (b *Board) appendPseudoLegalMoves(moves []Move) []Move {
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
				moves = b.genPawnMoves(sq, color, moves)
			case Knight:
				moves = b.genDeltaMoves(sq, color, knightDeltas, moves)
			case King:
				moves = b.genDeltaMoves(sq, color, kingDeltas, moves)
			case Rook:
				moves = b.genSliderMoves(sq, color, rookDirs, moves)
			case Bishop:
				moves = b.genSliderMoves(sq, color, bishopDirs, moves)
			case Queen:
				moves = b.genSliderMoves(sq, color, rookDirs, moves)
				moves = b.genSliderMoves(sq, color, bishopDirs, moves)
			}
		}
	}
	return b.genCastleMoves(color, moves)
}

// genDeltaMoves appends moves for a piece that moves by fixed deltas
// (knight, king): the target must be on-board and not occupied by a piece of
// the same color.
func (b *Board) genDeltaMoves(from Square, color Color, deltas [][2]int, moves []Move) []Move {
	for _, d := range deltas {
		to := Square{File: from.File + d[0], Rank: from.Rank + d[1]}
		if !to.Valid() {
			continue
		}
		target := b.PieceAt(to)
		if target.Type != NoPiece && target.Color == color {
			continue
		}
		moves = append(moves, Move{From: from, To: to})
	}
	return moves
}

// genSliderMoves appends moves for a sliding piece (rook/bishop/queen):
// walk each direction until the edge of the board, a friendly piece
// (excluded), or an enemy piece (included, ray stops there).
func (b *Board) genSliderMoves(from Square, color Color, dirs [][2]int, moves []Move) []Move {
	for _, d := range dirs {
		to := Square{File: from.File + d[0], Rank: from.Rank + d[1]}
		for to.Valid() {
			target := b.PieceAt(to)
			if target.Type == NoPiece {
				moves = append(moves, Move{From: from, To: to})
				to = Square{File: to.File + d[0], Rank: to.Rank + d[1]}
				continue
			}
			if target.Color != color {
				moves = append(moves, Move{From: from, To: to})
			}
			break
		}
	}
	return moves
}

// genPawnMoves appends single/double pushes, diagonal captures, en passant
// captures, and promotion expansions for the pawn on sq.
func (b *Board) genPawnMoves(sq Square, color Color, moves []Move) []Move {
	dir, startRank, lastRank, epRank := pawnGeometry(color)

	// Single push.
	one := Square{File: sq.File, Rank: sq.Rank + dir}
	if one.Valid() && b.PieceAt(one).Type == NoPiece {
		moves = addPawnMove(sq, one, lastRank, moves)

		// Double push, only from the starting rank and only if both the
		// intermediate and destination squares are empty.
		if sq.Rank == startRank {
			two := Square{File: sq.File, Rank: sq.Rank + 2*dir}
			if b.PieceAt(two).Type == NoPiece {
				moves = append(moves, Move{From: sq, To: two})
			}
		}
	}

	// Diagonal captures (including en passant).
	for _, df := range [2]int{-1, 1} {
		to := Square{File: sq.File + df, Rank: sq.Rank + dir}
		if !to.Valid() {
			continue
		}
		target := b.PieceAt(to)
		if target.Type != NoPiece && target.Color != color {
			moves = addPawnMove(sq, to, lastRank, moves)
			continue
		}
		if b.HasEP && to == b.EPSquare && to.Rank == epRank && target.Type == NoPiece {
			moves = append(moves, Move{From: sq, To: to})
		}
	}
	return moves
}

// pawnGeometry returns the ranks color's pawns move by: the direction
// they advance in, the rank they start on, the rank they promote on, and
// the rank an en passant capture lands on (White's sixth, Black's third).
// map.c generate_pawn_moves "rejects ep attempts for the colour that has
// just moved" with that last rule, which matters once a null move passes
// the turn without clearing the square ("1. e4 --" leaves e3 set with
// White to move): White's d2/f2 pawns must not capture onto it.
func pawnGeometry(color Color) (dir, startRank, lastRank, epRank int) {
	if color == Black {
		return -1, 6, 0, 2
	}
	return 1, 1, 7, 5
}

// addPawnMove appends the pawn move from->to (or, if the destination is on
// the last rank, its four promotion expansions).
func addPawnMove(from, to Square, lastRank int, moves []Move) []Move {
	if to.Rank == lastRank {
		for _, pt := range promotionPieces {
			moves = append(moves, Move{From: from, To: to, Promotion: pt})
		}
		return moves
	}
	return append(moves, Move{From: from, To: to})
}

// CastleMove returns the side to move's castling move to side and whether
// it is legal: exactly the move AppendLegalMoves lists for that side, if
// any, found without generating the position's other moves. Castling
// text names nothing but the side, so this is all a SAN "O-O" or "O-O-O"
// needs. It allocates nothing.
func (b *Board) CastleMove(side CastleSide) (Move, bool) {
	var buf [1]Move
	moves := b.keepLegal(b.appendCastleMove(b.SideToMove, side, buf[:0]), 0)
	if len(moves) == 0 {
		return Move{}, false
	}
	return moves[0], true
}

// genCastleMoves appends the legal castling moves for color, kingside
// first.
func (b *Board) genCastleMoves(color Color, moves []Move) []Move {
	moves = b.appendCastleMove(color, Kingside, moves)
	return b.appendCastleMove(color, Queenside, moves)
}

// appendCastleMove appends color's castling move to side if color holds
// the right and the move is pseudo-legal (castleMove's path and
// attacked-square rules), and returns the extended slice.
func (b *Board) appendCastleMove(color Color, side CastleSide, moves []Move) []Move {
	var right bool
	var rookFile, kingToFile, rookToFile int
	switch {
	case color == White && side == Kingside:
		right, rookFile = b.Castling.WhiteKingside, b.Castling.WhiteKRookFile
	case color == White:
		right, rookFile = b.Castling.WhiteQueenside, b.Castling.WhiteQRookFile
	case side == Kingside:
		right, rookFile = b.Castling.BlackKingside, b.Castling.BlackKRookFile
	default:
		right, rookFile = b.Castling.BlackQueenside, b.Castling.BlackQRookFile
	}
	if side == Kingside {
		kingToFile, rookToFile = 6, 5
	} else {
		kingToFile, rookToFile = 2, 3
	}
	if !right {
		return moves
	}
	if m, ok := b.castleMove(color, rookFile, side, kingToFile, rookToFile); ok {
		moves = append(moves, m)
	}
	return moves
}

// castleMove returns color's castling move to side, whose rook starts on
// rookFile and whose king and rook land on kingToFile and rookToFile, and
// whether it is legal. Generic over rook start files so it also works for
// Chess960: the squares that must be vacant are the union of the king's
// path and the rook's path, excluding the king and rook's own starting
// squares; the king must not start in check, and every square it passes
// through (inclusive of its destination) must not be attacked by the
// opponent.
func (b *Board) castleMove(color Color, rookFile int, side CastleSide, kingToFile, rookToFile int) (Move, bool) {
	kingSq := b.KingSquare(color)
	backRank := 0
	if color == Black {
		backRank = 7
	}
	rookSq := Square{File: rookFile, Rank: backRank}
	kingTo := Square{File: kingToFile, Rank: backRank}

	// All squares in the king's path and the rook's path must be empty,
	// except for the king's and rook's own starting squares.
	if !b.pathClear(kingSq.File, kingToFile, backRank, kingSq, rookSq) ||
		!b.pathClear(rookFile, rookToFile, backRank, kingSq, rookSq) {
		return Move{}, false
	}

	// The king may not be in check, nor cross (inclusive of its
	// destination) any square attacked by the opponent.
	opponent := color.Opponent()
	lo, hi := fileRange(kingSq.File, kingToFile)
	for f := lo; f <= hi; f++ {
		if b.IsAttacked(Square{File: f, Rank: backRank}, opponent) {
			return Move{}, false
		}
	}
	return Move{From: kingSq, To: kingTo, Castle: side}, true
}

// pathClear reports whether every square on rank between files fromFile and
// toFile (inclusive, in either order) is empty, ignoring the two squares
// a castling king and rook start on.
func (b *Board) pathClear(fromFile, toFile, rank int, kingSq, rookSq Square) bool {
	lo, hi := fileRange(fromFile, toFile)
	for f := lo; f <= hi; f++ {
		s := Square{File: f, Rank: rank}
		if s == kingSq || s == rookSq {
			continue
		}
		if b.PieceAt(s).Type != NoPiece {
			return false
		}
	}
	return true
}

// fileRange orders two files so a walk from lo to hi inclusive covers the
// squares between them whichever way the piece travels.
func fileRange(a, b int) (lo, hi int) {
	if a > b {
		return b, a
	}
	return a, b
}
