package replay

import (
	"testing"

	"github.com/kyleboon/pgnx/chess"
)

// TestPositionHashesMatchRebuiltBoards replays the bench bundle (real
// games with castling, checks, promotions and nested variations) and
// checks every recorded position's placement key and Zobrist hash against
// a board rebuilt from that position's FEN. The replay's board carries its
// placement key incrementally across Apply calls; the rebuilt board seeds
// its key from scratch, so the two agree only if every kind of move
// updates the key correctly.
func TestPositionHashesMatchRebuiltBoards(t *testing.T) {
	games := loadBenchGames(t)
	plies := 0
	for i, g := range games {
		r, err := Game(g, Options{})
		if err != nil {
			t.Fatalf("game %d: %v", i+1, err)
		}
		plies += checkLineHashes(t, r, i+1)
	}
	if plies < 3000 {
		t.Fatalf("checked only %d plies; the bench bundle should be larger", plies)
	}
}

func checkLineHashes(t *testing.T, r *Result, game int) int {
	t.Helper()
	b := r.StartBoard()
	n := 0
	for i, p := range r.Positions {
		if i > 0 {
			b.Apply(r.Moves[i-1].Move)
			n++
		}
		fen := b.FEN()
		rebuilt, err := chess.FromFEN(fen)
		if err != nil {
			t.Fatalf("game %d position %d: %v", game, i, err)
		}
		if p.Placement != rebuilt.PlacementKey() {
			t.Fatalf("game %d position %d (%s): Placement %#x, rebuilt %#x", game, i, fen, p.Placement, rebuilt.PlacementKey())
		}
		if p.Zobrist != rebuilt.Zobrist() {
			t.Fatalf("game %d position %d (%s): Zobrist %#x, rebuilt %#x", game, i, fen, p.Zobrist, rebuilt.Zobrist())
		}
		if i > 0 {
			for _, v := range r.Moves[i-1].Variations {
				if v == nil {
					t.Fatalf("game %d position %d: a variation failed to replay", game, i)
				}
				n += checkLineHashes(t, v, game)
			}
		}
	}
	return n
}
