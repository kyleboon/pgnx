package output

import (
	"testing"

	"github.com/kyleboon/pgnx/chess"
)

// TestCountMovesCountsAPromotionOnce pins the mobility count behind
// --evaluation to the C tool's find_all_moves: a pawn's promotion to one
// square counts once whatever the piece, however the generator orders
// the promotion moves. The white pawn on b7 promotes by push to b8 and
// by capture to a8 and c8 (three moves, twelve generated), and the king
// on e1 has five moves.
func TestCountMovesCountsAPromotionOnce(t *testing.T) {
	b, err := chess.FromFEN("n1n1k3/1P6/8/8/8/8/8/4K3 w - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	if got := countMoves(b, chess.White); got != 8 {
		t.Errorf("countMoves(White) = %d, want 8", got)
	}
	// Black as if to move: five king moves and the knights' two and four.
	if got := countMoves(b, chess.Black); got != 11 {
		t.Errorf("countMoves(Black) = %d, want 11", got)
	}
}
