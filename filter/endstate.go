package filter

import (
	"github.com/kyleboon/pgnx/chess"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// markComment returns the text of a --markmatches comment for a position:
// the literal mark, or the position's FEN when the mark is "FEN" (apply.c
// create_match_comment). board is called only for a FEN mark, so a caller
// that marks one position passes a replay.Result.BoardAt closure, and
// one that may mark every ply walks a board alongside the positions and
// passes that.
func markComment(mark string, board func() *chess.Board) string {
	if mark == "FEN" {
		return board().FEN()
	}
	return mark
}

// markBoard returns the board a loop over a line's positions walks for
// its FEN mark comments: r's start board when mark is "FEN", else nil.
// The caller applies each replayed move to it before testing the
// position the move reaches.
func markBoard(mark string, r *replay.Result) *chess.Board {
	if mark != "FEN" {
		return nil
	}
	return r.StartBoard()
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
	b := r.FinalBoard()
	return b != nil && b.IsStalemate()
}

// matchInsufficient reports whether the final position has insufficient
// mating material by pgn-extract's definition.
func matchInsufficient(r *replay.Result) bool {
	b := r.FinalBoard()
	return b != nil && b.InsufficientMaterial()
}

// matchNMoveRule reports whether the N-move rule (N = 50 or 75) was
// available at any point in the main line: after some ply the halfmove
// clock reached 2N (apply.c play_moves). Only moves before index stop
// whose ply number is at least startPly are examined, as in the C walk.
// With a non-empty mark, a comment is appended to every qualifying ply's
// move, not just the first.
func matchNMoveRule(g *pgn.Game, r *replay.Result, n int, mark string, startPly, stop int) bool {
	matched := false
	p0 := firstPly(r)
	b := markBoard(mark, r)
	for i := 0; i < stop && i < len(r.Moves); i++ {
		if i+1 >= len(r.Positions) || i >= len(g.Moves) {
			break
		}
		if b != nil {
			b.Apply(r.Moves[i].Move)
		}
		if p0+i < startPly {
			continue
		}
		pos := r.Positions[i+1]
		if pos.HalfmoveClock >= 2*n {
			matched = true
			if mark != "" {
				g.Moves[i].AddComment(markComment(mark, func() *chess.Board { return b }))
			}
		}
	}
	return matched
}

// firstPly is the ply number of the first move of r (1 for a game from
// the standard start), or 1 for an empty result.
func firstPly(r *replay.Result) int {
	if len(r.Positions) == 0 {
		return 1
	}
	return r.Positions[0].PlyNumber()
}

// matchRepetition reports whether some position occurs at least n times
// in the main line, counting the starting position (apply.c
// apply_move_list seeds the count list with the initial board). Only
// positions after moves before index stop whose ply number is at least
// startPly are counted (play_moves' check_for_match). Every ply at which
// a position's count reaches or exceeds n is marked.
//
// Position identity is the Polyglot Zobrist hash, which already folds
// in the side to move, castling rights, and the en passant file only
// when a capture is actually possible — exactly hashing.c's
// repetition_position_matches (board hash + to_move + castling rights +
// non-redundant ep square).
func matchRepetition(g *pgn.Game, r *replay.Result, n int, mark string, startPly, stop int) bool {
	if len(r.Positions) == 0 {
		return false
	}
	counts := make(map[uint64]int, len(r.Positions))
	counts[r.Positions[0].Zobrist]++
	matched := false
	p0 := firstPly(r)
	b := markBoard(mark, r)
	for i := 0; i < stop && i < len(r.Moves); i++ {
		if i+1 >= len(r.Positions) || i >= len(g.Moves) {
			break
		}
		if b != nil {
			b.Apply(r.Moves[i].Move)
		}
		if p0+i < startPly {
			continue
		}
		pos := r.Positions[i+1]
		counts[pos.Zobrist]++
		if counts[pos.Zobrist] >= n {
			matched = true
			if mark != "" {
				g.Moves[i].AddComment(markComment(mark, func() *chess.Board { return b }))
			}
		}
	}
	return matched
}

// matchUnderpromotion reports whether any main-line move before index
// stop, at a ply number of at least startPly, promoted to a piece other
// than a queen (apply.c play_moves).
func matchUnderpromotion(r *replay.Result, startPly, stop int) bool {
	p0 := firstPly(r)
	for i := 0; i < stop && i < len(r.Moves); i++ {
		if p0+i < startPly {
			continue
		}
		p := r.Moves[i].Move.Promotion
		if p != chess.NoPiece && p != chess.Queen {
			return true
		}
	}
	return false
}

// matchCommented reports whether the game has a prefix comment or, at any
// variation depth, a move with a leading comment (grammar.c
// check_for_comments, whose comments_in_move_list tests only each move's
// comment_list: a comment that follows a NAG or a variation does not
// count, and neither does a variation's prefix comment).
func matchCommented(g *pgn.Game) bool {
	if len(g.PrefixComments) > 0 {
		return true
	}
	return movesCommented(g.Moves)
}

func movesCommented(moves []*pgn.Move) bool {
	for _, m := range moves {
		if len(m.LeadingComments()) > 0 {
			return true
		}
		for _, v := range m.Variations() {
			if movesCommented(v.Moves) {
				return true
			}
		}
	}
	return false
}
