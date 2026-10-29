package filter

// Bounds restricts a game's length in plies, inclusive at both ends
// (moves.c check_move_bounds). MaxPly == 0 means no upper bound. The
// ply count compared is replay.Result.PlyCount, i.e. pgn-extract's
// final-board definition.
type Bounds struct {
	MinPly, MaxPly int
}

// The Set* helpers mirror argsfile.c set_move_bounds: a bound given in
// full moves (-b) converts to plies as 2*(n-1)+1 for the lower bound
// (the first ply of move n) and 2n for the upper (the last ply of move
// n), while -p bounds are plies already. SetMoves/SetPly set both.

// SetMinMoves sets the lower bound to the first ply of move n (-bl n).
func (b *Bounds) SetMinMoves(n int) { b.MinPly = 2*(n-1) + 1 }

// SetMaxMoves sets the upper bound to the last ply of move n (-bu n).
func (b *Bounds) SetMaxMoves(n int) { b.MaxPly = 2 * n }

// SetMoves requires exactly n full moves (-b n / -be n).
func (b *Bounds) SetMoves(n int) { b.SetMinMoves(n); b.SetMaxMoves(n) }

// SetMinPly sets the lower bound in plies (-pl n).
func (b *Bounds) SetMinPly(n int) { b.MinPly = n }

// SetMaxPly sets the upper bound in plies (-pu n).
func (b *Bounds) SetMaxPly(n int) { b.MaxPly = n }

// SetPly requires exactly n plies (-p n / -pe n).
func (b *Bounds) SetPly(n int) { b.MinPly, b.MaxPly = n, n }

// Match reports whether plyCount lies within the bounds.
func (b Bounds) Match(plyCount int) bool {
	if plyCount < b.MinPly {
		return false
	}
	return b.MaxPly == 0 || plyCount <= b.MaxPly
}
