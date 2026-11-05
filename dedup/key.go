package dedup

import "github.com/kyleboon/pgnx/replay"

// Key is a game's identity for duplicate detection: the placement key of
// the final main-line position and the wrapping sum of the placement
// keys after each main-line move (apply.c play_moves: final_hash_value
// and cumulative_hash_value). Two games with the same moves share a Key;
// a transposition shares Final only.
type Key struct {
	Final, Cumulative uint64
}

// KeyOf returns r's Key. A game with no moves has the start placement as
// Final and a Cumulative of 0; a nil or empty replay has the zero Key.
func KeyOf(r *replay.Result) Key {
	if r == nil || len(r.Positions) == 0 {
		return Key{}
	}
	var k Key
	for _, p := range r.Positions[1:] {
		k.Cumulative += p.Placement
	}
	k.Final = r.Positions[len(r.Positions)-1].Placement
	return k
}

// FuzzyKey returns the placement key r reaches after the move whose
// absolute ply number is depth (replay.Position.PlyNumber), or Final
// when depth is 0 (apply.c play_moves: fuzzy_duplicate_hash). ok is
// false when r has no such ply: the game is too short, or starts from a
// FEN beyond it.
func FuzzyKey(r *replay.Result, depth int) (key uint64, ok bool) {
	if r == nil || len(r.Positions) == 0 {
		return 0, false
	}
	if depth == 0 {
		return r.Positions[len(r.Positions)-1].Placement, true
	}
	// Positions[i] is the position the move with ply number
	// Positions[i].PlyNumber() is played from; the move's result is
	// Positions[i+1].
	i := depth - r.Positions[0].PlyNumber()
	if i < 0 || i+1 >= len(r.Positions) {
		return 0, false
	}
	return r.Positions[i+1].Placement, true
}
