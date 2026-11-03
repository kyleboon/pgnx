package output

import "github.com/kyleboon/pgnx/chess"

// shannonEvaluation is apply.c shannonEvaluation, the value --evaluation
// prints after each move: material (pawn 1, knight and bishop 3, rook 5,
// queen 9) plus a tenth of a point per move of mobility, White's minus
// Black's. Mobility counts each side's legal moves as if it were that
// side's turn, a promotion once whatever the piece, and an en passant
// capture only for the side the en passant square belongs to.
func shannonEvaluation(b *chess.Board) float64 {
	material := 0
	for file := 0; file < 8; file++ {
		for rank := 0; rank < 8; rank++ {
			p := b.PieceAt(chess.Square{File: file, Rank: rank})
			value := 0
			switch p.Type {
			case chess.Pawn:
				value = 1
			case chess.Knight, chess.Bishop:
				value = 3
			case chess.Rook:
				value = 5
			case chess.Queen:
				value = 9
			}
			if p.Color == chess.White {
				material += value
			} else {
				material -= value
			}
		}
	}
	mobility := countMoves(b, chess.White) - countMoves(b, chess.Black)
	return float64(material) + float64(mobility)*0.1
}

// countMoves counts c's legal moves on b as the C tool's find_all_moves
// does: as if c were to move, with the en passant square usable only by
// the side actually to move, and the promotion moves to one square
// counted once.
func countMoves(b *chess.Board, c chess.Color) int {
	board := b.Copy()
	if board.SideToMove != c {
		board.SideToMove = c
		board.HasEP = false
	}
	n := 0
	var lastFrom, lastTo chess.Square
	for i, m := range board.LegalMoves() {
		if m.Promotion != chess.NoPiece && i > 0 && m.From == lastFrom && m.To == lastTo {
			continue
		}
		lastFrom, lastTo = m.From, m.To
		n++
	}
	return n
}
