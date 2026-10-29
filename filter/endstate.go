package filter

import (
	"github.com/kyleboon/pgnx/chess"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// markComment returns the text of a --markmatches comment for a position:
// the literal mark, or the position's FEN when the mark is "FEN"
// (apply.c create_match_comment).
func markComment(mark string, pos replay.Position) string {
	if mark == "FEN" {
		return pos.FEN
	}
	return mark
}

// matchCheckmate reports whether any main-line move delivered mate
// (moves.c check_for_only_checkmate; in a legal game only the last move
// can).
func matchCheckmate(r *replay.Result) bool {
	for i := range r.Moves {
		if r.Moves[i].IsMate {
			return true
		}
	}
	return false
}

// matchStalemate reports whether the final position is stalemate
// (moves.c is_stalemate): the last move must not have given check and
// the side to move must have no legal move.
func matchStalemate(r *replay.Result) bool {
	if n := len(r.Moves); n > 0 && r.Moves[n-1].IsCheck {
		return false
	}
	b, err := r.FinalBoard()
	if err != nil {
		return false
	}
	return b.IsStalemate()
}

// matchInsufficient reports whether the final position has insufficient
// mating material by pgn-extract's definition.
func matchInsufficient(r *replay.Result) bool {
	b, err := r.FinalBoard()
	if err != nil {
		return false
	}
	return b.InsufficientMaterial()
}

// matchNMoveRule reports whether the N-move rule (N = 50 or 75) was
// available at any point in the main line: after some ply the halfmove
// clock reached 2N (apply.c play_moves). With a non-empty mark, a
// comment is appended to every qualifying ply's move, not just the
// first.
func matchNMoveRule(g *pgn.Game, r *replay.Result, n int, mark string) bool {
	matched := false
	for i := range r.Moves {
		if i+1 >= len(r.Positions) || i >= len(g.Moves) {
			break
		}
		pos := r.Positions[i+1]
		if pos.HalfmoveClock >= 2*n {
			matched = true
			if mark != "" {
				g.Moves[i].Comments = append(g.Moves[i].Comments, markComment(mark, pos))
			}
		}
	}
	return matched
}

// matchRepetition reports whether some position occurs at least n times
// in the main line, counting the starting position (apply.c
// apply_move_list seeds the count list with the initial board). Every
// ply at which a position's count reaches or exceeds n is marked.
//
// Position identity is the Polyglot Zobrist hash, which already folds
// in the side to move, castling rights, and the en passant file only
// when a capture is actually possible — exactly hashing.c's
// repetition_position_matches (board hash + to_move + castling rights +
// non-redundant ep square).
func matchRepetition(g *pgn.Game, r *replay.Result, n int, mark string) bool {
	if len(r.Positions) == 0 {
		return false
	}
	counts := make(map[uint64]int, len(r.Positions))
	counts[r.Positions[0].Zobrist]++
	matched := false
	for i := range r.Moves {
		if i+1 >= len(r.Positions) || i >= len(g.Moves) {
			break
		}
		pos := r.Positions[i+1]
		counts[pos.Zobrist]++
		if counts[pos.Zobrist] >= n {
			matched = true
			if mark != "" {
				g.Moves[i].Comments = append(g.Moves[i].Comments, markComment(mark, pos))
			}
		}
	}
	return matched
}

// matchUnderpromotion reports whether any main-line move promoted to a
// piece other than a queen (apply.c play_moves).
func matchUnderpromotion(r *replay.Result) bool {
	for i := range r.Moves {
		p := r.Moves[i].Move.Promotion
		if p != chess.NoPiece && p != chess.Queen {
			return true
		}
	}
	return false
}

// matchCommented reports whether the game has a prefix comment or any
// move comment at any variation depth (grammar.c check_for_comments).
func matchCommented(g *pgn.Game) bool {
	if len(g.PrefixComments) > 0 {
		return true
	}
	return movesCommented(g.Moves)
}

func movesCommented(moves []*pgn.Move) bool {
	for _, m := range moves {
		if len(m.Comments) > 0 {
			return true
		}
		for _, v := range m.Variations {
			if movesCommented(v.Moves) {
				return true
			}
		}
	}
	return false
}
