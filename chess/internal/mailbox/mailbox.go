// Package mailbox is the chess package's previous board engine, an
// [8][8]chess.Piece mailbox probed square by square as pgn-extract's map.c
// walks its board, kept as the oracle for the bitboard engine (ADR
// 0005). It is imported only by the chess package's differential test,
// which replays corpora with both engines and compares them position by
// position. It does not maintain hashes; the test computes the
// mailbox's placement key and weak hash by scanning its squares.
package mailbox

import (
	"strconv"
	"strings"

	"github.com/kyleboon/pgnx/chess"
)

// Board is the mailbox position: the same exported fields as chess.Board.
type Board struct {
	SideToMove     chess.Color
	Castling       chess.CastlingRights
	HasEP          bool
	EPSquare       chess.Square
	HalfmoveClock  int
	FullmoveNumber int

	squares [8][8]chess.Piece // indexed [file][rank]
	kings   [3]chess.Square   // indexed by chess.Color
}

// FromBoard builds the mailbox position of b, piece by piece.
func FromBoard(b *chess.Board) *Board {
	mb := &Board{
		SideToMove:     b.SideToMove,
		Castling:       b.Castling,
		HasEP:          b.HasEP,
		EPSquare:       b.EPSquare,
		HalfmoveClock:  b.HalfmoveClock,
		FullmoveNumber: b.FullmoveNumber,
	}
	for f := 0; f < 8; f++ {
		for r := 0; r < 8; r++ {
			sq := chess.Square{File: f, Rank: r}
			mb.setPiece(sq, b.PieceAt(sq))
		}
	}
	return mb
}

// PieceAt returns the piece on the given square, or the zero chess.Piece if empty.
func (b *Board) PieceAt(sq chess.Square) chess.Piece {
	return b.squares[sq.File][sq.Rank]
}

func (b *Board) setPiece(sq chess.Square, p chess.Piece) {
	old := b.squares[sq.File][sq.Rank]
	if old.Type == chess.King && b.kings[old.Color] == sq {
		b.kings[old.Color] = chess.Square{}
	}
	b.squares[sq.File][sq.Rank] = p
	if p.Type == chess.King {
		b.kings[p.Color] = sq
	}
}

// KingSquare returns the cached square of the given color's king.
func (b *Board) KingSquare(c chess.Color) chess.Square {
	return b.kings[c]
}

// Copy returns an independent copy of the board.
func (b *Board) Copy() *Board {
	nb := *b
	return &nb
}

var (
	knightDeltas = [][2]int{{1, 2}, {2, 1}, {2, -1}, {1, -2}, {-1, -2}, {-2, -1}, {-2, 1}, {-1, 2}}
	kingDeltas   = [][2]int{{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}}
	rookDirs     = [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	bishopDirs   = [][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
)

// IsAttacked reports whether sq is attacked by any piece of color by,
// considering knight, king, pawn, and sliding (rook/bishop/queen) attacks.
// It does not consider whose turn it is to move.
func (b *Board) IsAttacked(sq chess.Square, by chess.Color) bool {
	for _, d := range knightDeltas {
		if b.pieceIs(chess.Square{File: sq.File + d[0], Rank: sq.Rank + d[1]}, chess.Knight, by) {
			return true
		}
	}
	for _, d := range kingDeltas {
		if b.pieceIs(chess.Square{File: sq.File + d[0], Rank: sq.Rank + d[1]}, chess.King, by) {
			return true
		}
	}
	// Pawns: a white pawn on rank r attacks r+1, so look one rank *toward* the attacker.
	pr := -1
	if by == chess.White {
		pr = 1
	}
	for _, df := range []int{-1, 1} {
		if b.pieceIs(chess.Square{File: sq.File + df, Rank: sq.Rank - pr}, chess.Pawn, by) {
			return true
		}
	}
	if b.rayHits(sq, rookDirs, by, chess.Rook) || b.rayHits(sq, bishopDirs, by, chess.Bishop) {
		return true
	}
	return false
}

func (b *Board) rayHits(sq chess.Square, dirs [][2]int, by chess.Color, slider chess.PieceType) bool {
	for _, d := range dirs {
		cur := chess.Square{File: sq.File + d[0], Rank: sq.Rank + d[1]}
		for cur.Valid() {
			p := b.PieceAt(cur)
			if p.Type != chess.NoPiece {
				if p.Color == by && (p.Type == slider || p.Type == chess.Queen) {
					return true
				}
				break
			}
			cur = chess.Square{File: cur.File + d[0], Rank: cur.Rank + d[1]}
		}
	}
	return false
}

func (b *Board) pieceIs(sq chess.Square, pt chess.PieceType, c chess.Color) bool {
	return sq.Valid() && b.PieceAt(sq) == chess.Piece{Type: pt, Color: c}
}

// InCheck reports whether color c's king is currently attacked.
func (b *Board) InCheck(c chess.Color) bool {
	return b.IsAttacked(b.KingSquare(c), c.Opponent())
}

// promotionPieces lists the piece types a pawn may promote to, in the order
// promotion moves are generated.
var promotionPieces = []chess.PieceType{chess.Queen, chess.Rook, chess.Bishop, chess.Knight}

// LegalMoves returns all fully legal moves for the side to move: pseudo-legal
// moves that do not leave the mover's own king in check. It allocates a
// fresh slice per call; a caller generating repeatedly keeps one buffer
// and uses AppendLegalMoves instead.
func (b *Board) LegalMoves() []chess.Move {
	return b.AppendLegalMoves(make([]chess.Move, 0, legalMovesHint))
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
func (b *Board) AppendLegalMoves(moves []chess.Move) []chess.Move {
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
func (b *Board) AppendLegalMovesTo(pt chess.PieceType, to chess.Square, moves []chess.Move) []chess.Move {
	start := len(moves)
	color := b.SideToMove
	switch pt {
	case chess.Pawn:
		moves = b.findPawnMoves(to, color, moves)
	case chess.Knight:
		moves = b.findDeltaMoves(to, chess.Knight, color, knightDeltas, moves)
	case chess.King:
		moves = b.findDeltaMoves(to, chess.King, color, kingDeltas, moves)
	case chess.Rook:
		moves = b.findSliderMoves(to, chess.Rook, color, rookDirs, moves)
	case chess.Bishop:
		moves = b.findSliderMoves(to, chess.Bishop, color, bishopDirs, moves)
	case chess.Queen:
		moves = b.findSliderMoves(to, chess.Queen, color, rookDirs, moves)
		moves = b.findSliderMoves(to, chess.Queen, color, bishopDirs, moves)
	}
	return b.keepLegal(moves, start)
}

// keepLegal filters moves[start:] in place to the pseudo-legal moves
// whose making leaves the mover's king unattacked, compacting them
// towards start, and returns the shortened slice. The legality probe is
// a stack copy of the board, so no candidate allocates.
func (b *Board) keepLegal(moves []chess.Move, start int) []chess.Move {
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
func (b *Board) findDeltaMoves(to chess.Square, pt chess.PieceType, color chess.Color, deltas [][2]int, moves []chess.Move) []chess.Move {
	if target := b.PieceAt(to); target.Type != chess.NoPiece && target.Color == color {
		return moves
	}
	for _, d := range deltas {
		from := chess.Square{File: to.File - d[0], Rank: to.Rank - d[1]}
		if b.pieceIs(from, pt, color) {
			moves = append(moves, chess.Move{From: from, To: to})
		}
	}
	return moves
}

// findSliderMoves appends the moves to to of color's sliding pieces of
// type pt: along each direction the first piece met, if it is such a
// piece, has a clear ray to to. Nothing is found when to holds a piece of
// the same color.
func (b *Board) findSliderMoves(to chess.Square, pt chess.PieceType, color chess.Color, dirs [][2]int, moves []chess.Move) []chess.Move {
	if target := b.PieceAt(to); target.Type != chess.NoPiece && target.Color == color {
		return moves
	}
	for _, d := range dirs {
		from := chess.Square{File: to.File + d[0], Rank: to.Rank + d[1]}
		for from.Valid() {
			p := b.PieceAt(from)
			if p.Type == chess.NoPiece {
				from = chess.Square{File: from.File + d[0], Rank: from.Rank + d[1]}
				continue
			}
			if p.Type == pt && p.Color == color {
				moves = append(moves, chess.Move{From: from, To: to})
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
func (b *Board) findPawnMoves(to chess.Square, color chess.Color, moves []chess.Move) []chess.Move {
	dir, startRank, lastRank, epRank := pawnGeometry(color)

	target := b.PieceAt(to)
	if target.Type == chess.NoPiece {
		// Pushes: the pawn one square back, or two squares back from
		// its starting rank with the square between empty.
		one := chess.Square{File: to.File, Rank: to.Rank - dir}
		if one.Valid() {
			switch {
			case b.pieceIs(one, chess.Pawn, color):
				moves = addPawnMove(one, to, lastRank, moves)
			case b.PieceAt(one).Type == chess.NoPiece && to.Rank == startRank+2*dir:
				two := chess.Square{File: to.File, Rank: startRank}
				if b.pieceIs(two, chess.Pawn, color) {
					moves = append(moves, chess.Move{From: two, To: to})
				}
			}
		}
		if !(b.HasEP && to == b.EPSquare && to.Rank == epRank) {
			return moves
		}
		// En passant: the capturing pawns sit diagonally behind to.
		for _, df := range [2]int{-1, 1} {
			from := chess.Square{File: to.File + df, Rank: to.Rank - dir}
			if b.pieceIs(from, chess.Pawn, color) {
				moves = append(moves, chess.Move{From: from, To: to})
			}
		}
		return moves
	}
	if target.Color == color {
		return moves
	}
	for _, df := range [2]int{-1, 1} {
		from := chess.Square{File: to.File + df, Rank: to.Rank - dir}
		if b.pieceIs(from, chess.Pawn, color) {
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
	var buf [stackMoveBuffer]chess.Move
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
func (b *Board) appendPseudoLegalMoves(moves []chess.Move) []chess.Move {
	color := b.SideToMove
	for file := 0; file < 8; file++ {
		for rank := 0; rank < 8; rank++ {
			sq := chess.Square{File: file, Rank: rank}
			p := b.PieceAt(sq)
			if p.Type == chess.NoPiece || p.Color != color {
				continue
			}
			switch p.Type {
			case chess.Pawn:
				moves = b.genPawnMoves(sq, color, moves)
			case chess.Knight:
				moves = b.genDeltaMoves(sq, color, knightDeltas, moves)
			case chess.King:
				moves = b.genDeltaMoves(sq, color, kingDeltas, moves)
			case chess.Rook:
				moves = b.genSliderMoves(sq, color, rookDirs, moves)
			case chess.Bishop:
				moves = b.genSliderMoves(sq, color, bishopDirs, moves)
			case chess.Queen:
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
func (b *Board) genDeltaMoves(from chess.Square, color chess.Color, deltas [][2]int, moves []chess.Move) []chess.Move {
	for _, d := range deltas {
		to := chess.Square{File: from.File + d[0], Rank: from.Rank + d[1]}
		if !to.Valid() {
			continue
		}
		target := b.PieceAt(to)
		if target.Type != chess.NoPiece && target.Color == color {
			continue
		}
		moves = append(moves, chess.Move{From: from, To: to})
	}
	return moves
}

// genSliderMoves appends moves for a sliding piece (rook/bishop/queen):
// walk each direction until the edge of the board, a friendly piece
// (excluded), or an enemy piece (included, ray stops there).
func (b *Board) genSliderMoves(from chess.Square, color chess.Color, dirs [][2]int, moves []chess.Move) []chess.Move {
	for _, d := range dirs {
		to := chess.Square{File: from.File + d[0], Rank: from.Rank + d[1]}
		for to.Valid() {
			target := b.PieceAt(to)
			if target.Type == chess.NoPiece {
				moves = append(moves, chess.Move{From: from, To: to})
				to = chess.Square{File: to.File + d[0], Rank: to.Rank + d[1]}
				continue
			}
			if target.Color != color {
				moves = append(moves, chess.Move{From: from, To: to})
			}
			break
		}
	}
	return moves
}

// genPawnMoves appends single/double pushes, diagonal captures, en passant
// captures, and promotion expansions for the pawn on sq.
func (b *Board) genPawnMoves(sq chess.Square, color chess.Color, moves []chess.Move) []chess.Move {
	dir, startRank, lastRank, epRank := pawnGeometry(color)

	// Single push.
	one := chess.Square{File: sq.File, Rank: sq.Rank + dir}
	if one.Valid() && b.PieceAt(one).Type == chess.NoPiece {
		moves = addPawnMove(sq, one, lastRank, moves)

		// Double push, only from the starting rank and only if both the
		// intermediate and destination squares are empty.
		if sq.Rank == startRank {
			two := chess.Square{File: sq.File, Rank: sq.Rank + 2*dir}
			if b.PieceAt(two).Type == chess.NoPiece {
				moves = append(moves, chess.Move{From: sq, To: two})
			}
		}
	}

	// Diagonal captures (including en passant).
	for _, df := range [2]int{-1, 1} {
		to := chess.Square{File: sq.File + df, Rank: sq.Rank + dir}
		if !to.Valid() {
			continue
		}
		target := b.PieceAt(to)
		if target.Type != chess.NoPiece && target.Color != color {
			moves = addPawnMove(sq, to, lastRank, moves)
			continue
		}
		if b.HasEP && to == b.EPSquare && to.Rank == epRank && target.Type == chess.NoPiece {
			moves = append(moves, chess.Move{From: sq, To: to})
		}
	}
	return moves
}

// pawnGeometry returns the ranks color's pawns move by: the direction
// they advance in, the rank they start on, the rank they promote on, and
// the rank an en passant capture lands on (chess.White's sixth, chess.Black's third).
// map.c generate_pawn_moves "rejects ep attempts for the colour that has
// just moved" with that last rule, which matters once a null move passes
// the turn without clearing the square ("1. e4 --" leaves e3 set with
// chess.White to move): chess.White's d2/f2 pawns must not capture onto it.
func pawnGeometry(color chess.Color) (dir, startRank, lastRank, epRank int) {
	if color == chess.Black {
		return -1, 6, 0, 2
	}
	return 1, 1, 7, 5
}

// addPawnMove appends the pawn move from->to (or, if the destination is on
// the last rank, its four promotion expansions).
func addPawnMove(from, to chess.Square, lastRank int, moves []chess.Move) []chess.Move {
	if to.Rank == lastRank {
		for _, pt := range promotionPieces {
			moves = append(moves, chess.Move{From: from, To: to, Promotion: pt})
		}
		return moves
	}
	return append(moves, chess.Move{From: from, To: to})
}

// CastleMove returns the side to move's castling move to side and whether
// it is legal: exactly the move AppendLegalMoves lists for that side, if
// any, found without generating the position's other moves. Castling
// text names nothing but the side, so this is all a SAN "O-O" or "O-O-O"
// needs. It allocates nothing.
func (b *Board) CastleMove(side chess.CastleSide) (chess.Move, bool) {
	var buf [1]chess.Move
	moves := b.keepLegal(b.appendCastleMove(b.SideToMove, side, buf[:0]), 0)
	if len(moves) == 0 {
		return chess.Move{}, false
	}
	return moves[0], true
}

// genCastleMoves appends the legal castling moves for color, kingside
// first.
func (b *Board) genCastleMoves(color chess.Color, moves []chess.Move) []chess.Move {
	moves = b.appendCastleMove(color, chess.Kingside, moves)
	return b.appendCastleMove(color, chess.Queenside, moves)
}

// appendCastleMove appends color's castling move to side if color holds
// the right and the move is pseudo-legal (castleMove's path and
// attacked-square rules), and returns the extended slice.
func (b *Board) appendCastleMove(color chess.Color, side chess.CastleSide, moves []chess.Move) []chess.Move {
	var right bool
	var rookFile, kingToFile, rookToFile int
	switch {
	case color == chess.White && side == chess.Kingside:
		right, rookFile = b.Castling.WhiteKingside, b.Castling.WhiteKRookFile
	case color == chess.White:
		right, rookFile = b.Castling.WhiteQueenside, b.Castling.WhiteQRookFile
	case side == chess.Kingside:
		right, rookFile = b.Castling.BlackKingside, b.Castling.BlackKRookFile
	default:
		right, rookFile = b.Castling.BlackQueenside, b.Castling.BlackQRookFile
	}
	if side == chess.Kingside {
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
func (b *Board) castleMove(color chess.Color, rookFile int, side chess.CastleSide, kingToFile, rookToFile int) (chess.Move, bool) {
	kingSq := b.KingSquare(color)
	backRank := 0
	if color == chess.Black {
		backRank = 7
	}
	rookSq := chess.Square{File: rookFile, Rank: backRank}
	kingTo := chess.Square{File: kingToFile, Rank: backRank}

	// All squares in the king's path and the rook's path must be empty,
	// except for the king's and rook's own starting squares.
	if !b.pathClear(kingSq.File, kingToFile, backRank, kingSq, rookSq) ||
		!b.pathClear(rookFile, rookToFile, backRank, kingSq, rookSq) {
		return chess.Move{}, false
	}

	// The king may not be in check, nor cross (inclusive of its
	// destination) any square attacked by the opponent.
	opponent := color.Opponent()
	lo, hi := fileRange(kingSq.File, kingToFile)
	for f := lo; f <= hi; f++ {
		if b.IsAttacked(chess.Square{File: f, Rank: backRank}, opponent) {
			return chess.Move{}, false
		}
	}
	return chess.Move{From: kingSq, To: kingTo, Castle: side}, true
}

// pathClear reports whether every square on rank between files fromFile and
// toFile (inclusive, in either order) is empty, ignoring the two squares
// a castling king and rook start on.
func (b *Board) pathClear(fromFile, toFile, rank int, kingSq, rookSq chess.Square) bool {
	lo, hi := fileRange(fromFile, toFile)
	for f := lo; f <= hi; f++ {
		s := chess.Square{File: f, Rank: rank}
		if s == kingSq || s == rookSq {
			continue
		}
		if b.PieceAt(s).Type != chess.NoPiece {
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

// Apply mutates the board by making move m. The caller must ensure m is
// pseudo-legal for the side to move (e.g. a move produced by LegalMoves or
// replayed from a game record); Apply does not validate legality.
//
// The null move (chess.NullMove) only passes the turn: the side to move flips
// and the fullmove number advances after chess.Black. The en passant square,
// the halfmove clock and the castling rights are left exactly as they
// were, as pgn-extract's apply_move skips make_move for a NULL_MOVE
// (apply.c), so the position after "1. e4 --" still carries e3.
func (b *Board) Apply(m chess.Move) {
	color := b.SideToMove
	if m.IsNull() {
		if color == chess.Black {
			b.FullmoveNumber++
		}
		b.SideToMove = color.Opponent()
		return
	}
	mover := b.PieceAt(m.From)

	// Step 1: en passant capture removes the pawn captured "in passing",
	// which sits beside the destination square rather than on it.
	if mover.Type == chess.Pawn && b.HasEP && m.To == b.EPSquare && b.PieceAt(m.To).Type == chess.NoPiece {
		b.setPiece(chess.Square{File: m.To.File, Rank: m.From.Rank}, chess.Piece{})
	}

	// Step 2: capture (if any) must be read before the board is mutated;
	// castling never captures.
	var captured chess.Piece
	if m.Castle == chess.NoCastle {
		captured = b.PieceAt(m.To)
	}

	if m.Castle != chess.NoCastle {
		// Step 3: castling. Compute rook start file from castling rights,
		// and the king/rook destination files, then clear both the king
		// and rook squares before placing either piece (their start/end
		// squares can coincide in Chess960).
		var rookFromFile, kingToFile, rookToFile int
		if color == chess.White {
			if m.Castle == chess.Kingside {
				rookFromFile = b.Castling.WhiteKRookFile
			} else {
				rookFromFile = b.Castling.WhiteQRookFile
			}
		} else {
			if m.Castle == chess.Kingside {
				rookFromFile = b.Castling.BlackKRookFile
			} else {
				rookFromFile = b.Castling.BlackQRookFile
			}
		}
		if m.Castle == chess.Kingside {
			kingToFile, rookToFile = 6, 5
		} else {
			kingToFile, rookToFile = 2, 3
		}

		rank := m.From.Rank
		rookFrom := chess.Square{File: rookFromFile, Rank: rank}
		kingTo := chess.Square{File: kingToFile, Rank: rank}
		rookTo := chess.Square{File: rookToFile, Rank: rank}

		rook := chess.Piece{Type: chess.Rook, Color: color}
		king := chess.Piece{Type: chess.King, Color: color}

		b.setPiece(m.From, chess.Piece{})
		b.setPiece(rookFrom, chess.Piece{})
		b.setPiece(kingTo, king)
		b.setPiece(rookTo, rook)
	} else {
		// Step 4: normal move (including captures and promotions): clear
		// the origin square and place the moving piece (or promoted piece)
		// on the destination square.
		b.setPiece(m.From, chess.Piece{})
		placed := mover
		if m.Promotion != chess.NoPiece {
			placed = chess.Piece{Type: m.Promotion, Color: color}
		}
		b.setPiece(m.To, placed)
	}

	// Step 5: castling-rights updates.
	if mover.Type == chess.King || m.Castle != chess.NoCastle {
		if color == chess.White {
			b.Castling.WhiteKingside = false
			b.Castling.WhiteQueenside = false
		} else {
			b.Castling.BlackKingside = false
			b.Castling.BlackQueenside = false
		}
	}
	if mover.Type == chess.Rook && m.Castle == chess.NoCastle {
		clearRookRight(b, color, m.From)
	}
	if m.Castle == chess.NoCastle && captured.Type == chess.Rook {
		clearRookRight(b, color.Opponent(), m.To)
	}

	// Step 6: en passant state is only set for a two-square pawn push.
	if mover.Type == chess.Pawn && abs(m.To.Rank-m.From.Rank) == 2 {
		b.HasEP = true
		b.EPSquare = chess.Square{File: m.From.File, Rank: (m.From.Rank + m.To.Rank) / 2}
	} else {
		b.HasEP = false
		b.EPSquare = chess.Square{}
	}

	// Step 7: clocks and side to move.
	if mover.Type == chess.Pawn || captured.Type != chess.NoPiece {
		b.HalfmoveClock = 0
	} else {
		b.HalfmoveClock++
	}
	if color == chess.Black {
		b.FullmoveNumber++
	}
	b.SideToMove = color.Opponent()
}

// clearRookRight clears owner's castling right whose rook-start square is
// sq, if that right is currently set and sq is on owner's back rank.
func clearRookRight(b *Board, owner chess.Color, sq chess.Square) {
	backRank := 0
	if owner == chess.Black {
		backRank = 7
	}
	if sq.Rank != backRank {
		return
	}
	if owner == chess.White {
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
	var counts [3][7]int
	var bishops []chess.Square
	for f := 0; f < 8; f++ {
		for r := 0; r < 8; r++ {
			p := b.squares[f][r]
			if p.Type == chess.NoPiece {
				continue
			}
			counts[p.Color][p.Type]++
			if p.Type == chess.Bishop {
				bishops = append(bishops, chess.Square{File: f, Rank: r})
			}
		}
	}
	w, bl := counts[chess.White], counts[chess.Black]
	if w[chess.Pawn] != 0 || bl[chess.Pawn] != 0 || w[chess.Rook] != 0 || bl[chess.Rook] != 0 || w[chess.Queen] != 0 || bl[chess.Queen] != 0 {
		return false
	}
	if w[chess.Bishop] > 1 || bl[chess.Bishop] > 1 {
		return false
	}
	if (w[chess.Bishop] != 0 && w[chess.Knight] != 0) || (bl[chess.Bishop] != 0 && bl[chess.Knight] != 0) {
		return false
	}
	if (w[chess.Knight] >= 2 && (bl[chess.Bishop] != 0 || bl[chess.Knight] != 0)) ||
		(bl[chess.Knight] >= 2 && (w[chess.Bishop] != 0 || w[chess.Knight] != 0)) {
		return false
	}
	if w[chess.Bishop] == 1 && bl[chess.Bishop] == 1 {
		// Opposite-colour bishops: odd Manhattan parity between them.
		d := abs(bishops[0].File-bishops[1].File) + abs(bishops[0].Rank-bishops[1].Rank)
		if d%2 == 1 {
			return false
		}
	}
	return true
}

// FEN returns the FEN string for the board.
func (b *Board) FEN() string {
	var sb strings.Builder

	for r := 7; r >= 0; r-- {
		empty := 0
		for f := 0; f < 8; f++ {
			p := b.squares[f][r]
			if p.Type == chess.NoPiece {
				empty++
				continue
			}
			if empty > 0 {
				sb.WriteString(strconv.Itoa(empty))
				empty = 0
			}
			sb.WriteRune(fenCharFromPiece(p))
		}
		if empty > 0 {
			sb.WriteString(strconv.Itoa(empty))
		}
		if r > 0 {
			sb.WriteByte('/')
		}
	}

	sb.WriteByte(' ')
	if b.SideToMove == chess.Black {
		sb.WriteByte('b')
	} else {
		sb.WriteByte('w')
	}

	sb.WriteByte(' ')
	sb.WriteString(castlingFEN(b.Castling))

	sb.WriteByte(' ')
	if b.HasEP {
		sb.WriteString(b.EPSquare.String())
	} else {
		sb.WriteByte('-')
	}

	sb.WriteByte(' ')
	sb.WriteString(strconv.Itoa(b.HalfmoveClock))
	sb.WriteByte(' ')
	sb.WriteString(strconv.Itoa(b.FullmoveNumber))

	return sb.String()
}

// castlingFEN spells the castling field as pgn-extract's
// build_basic_EPD_string (apply.c) does. A side with both rights is
// written K and Q (k and q) whatever files its rooks start on: X-FEN,
// where the letter names the outermost rook on that wing, which is what
// parseCastling resolves it back to. (The C tool writes both file
// letters instead when its kingside file is not the greater, which its
// own Shredder-letter parsing can produce; a chess.CastlingRights always has
// the kingside file greater, so pgnx has no such case.) A side with one right is written
// K or Q only when that rook starts on its standard file (h or a);
// otherwise the right is the rook's file letter, as in Shredder-FEN, so
// a field may mix the two styles ("Kd"), which parseCastling accepts.
// (A right for the inner of two rooks on one wing is therefore not
// round-tripped, as in the C tool.)
func castlingFEN(c chess.CastlingRights) string {
	var sb strings.Builder
	side := func(kingside, queenside bool, kFile, qFile int, k, q byte, a byte) {
		switch {
		case kingside && queenside:
			sb.WriteByte(k)
			sb.WriteByte(q)
		case kingside && kFile == 7:
			sb.WriteByte(k)
		case kingside:
			sb.WriteByte(a + byte(kFile))
		case queenside && qFile == 0:
			sb.WriteByte(q)
		case queenside:
			sb.WriteByte(a + byte(qFile))
		}
	}
	side(c.WhiteKingside, c.WhiteQueenside, c.WhiteKRookFile, c.WhiteQRookFile, 'K', 'Q', 'A')
	side(c.BlackKingside, c.BlackQueenside, c.BlackKRookFile, c.BlackQRookFile, 'k', 'q', 'a')
	if sb.Len() == 0 {
		return "-"
	}
	return sb.String()
}
func fenCharFromPiece(p chess.Piece) rune {
	var ch rune
	switch p.Type {
	case chess.Pawn:
		ch = 'p'
	case chess.Knight:
		ch = 'n'
	case chess.Bishop:
		ch = 'b'
	case chess.Rook:
		ch = 'r'
	case chess.Queen:
		ch = 'q'
	case chess.King:
		ch = 'k'
	}
	if p.Color == chess.White {
		ch = ch - 'a' + 'A'
	}
	return ch
}
