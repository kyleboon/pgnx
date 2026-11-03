package filter

import (
	"regexp"

	"github.com/kyleboon/pgnx/chess"
	"github.com/kyleboon/pgnx/pgn"
)

// oddsEmptyMiddle is grammar.c check_for_odds's test that the middle four
// ranks of the FEN are empty: pieces only on the first two ranks of each
// side.
var oddsEmptyMiddle = regexp.MustCompile(`^[^/]*/[^/]*/8/8/8/8/`)

// matchOdds is grammar.c check_for_odds: whether g appears to have been
// played at odds. The game must start from a FEN tag whose middle four
// ranks are empty, with White to move at move 1 (a fullmove field below
// 1 counts as 1, as new_fen_board reads it), and the two sides must
// differ in their number of pawns, knights, bishops, rooks or queens. A
// FEN that does not parse is not odds.
func matchOdds(g *pgn.Game) bool {
	fen, ok := g.Tag("FEN")
	if !ok || !oddsEmptyMiddle.MatchString(fen) {
		return false
	}
	b, err := chess.FromFEN(fen)
	if err != nil {
		return false
	}
	if b.FullmoveNumber > 1 || b.SideToMove != chess.White {
		return false
	}
	// Pieces per colour and type, indexed [chess.Color][chess.PieceType].
	var counts [3][7]int
	for file := 0; file < 8; file++ {
		for rank := 0; rank < 8; rank++ {
			p := b.PieceAt(chess.Square{File: file, Rank: rank})
			if p.Type != chess.NoPiece {
				counts[p.Color][p.Type]++
			}
		}
	}
	for _, pt := range []chess.PieceType{chess.Pawn, chess.Knight, chess.Bishop, chess.Rook, chess.Queen} {
		if counts[chess.White][pt] != counts[chess.Black][pt] {
			return true
		}
	}
	return false
}
