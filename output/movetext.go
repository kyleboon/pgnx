package output

import (
	"strconv"
	"strings"

	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// emitMovetext writes g's movetext (main line, recursively including
// variations) followed by the result token (unless opts.NoResults) to lw.
func emitMovetext(lw *lineWriter, g *pgn.Game, r *replay.Result, opts Options) {
	var moveInfos []replay.MoveInfo
	if r != nil {
		moveInfos = r.Moves
	}
	// The main line always starts with white to move.
	emitLine(lw, g.Moves, moveInfos, true, true, opts)

	if !opts.NoResults {
		result := g.Result
		if result == "" {
			result = "*"
		}
		lw.WriteToken(result)
	}
}

// emitLine writes one line of movetext (a main line or the body of a
// variation): moves is the pgn.Move sequence, infos is the parallel
// replay.MoveInfo sequence (nil/empty when replay data is unavailable,
// which is only valid for Format == Source). startWhite indicates whether
// moves[0] (if any) is white's move — true for the main line, and for a
// variation it matches the color of the move the variation is attached to
// (a RAV is an alternative to a single ply, so it begins on that same side
// to move). blackNeedsNumber tracks whether the very next move, if it is
// black's, must be given an explicit "N..." number because of a preceding
// interruption (comment/variation) or because this line starts with black
// to move.
func emitLine(lw *lineWriter, moves []*pgn.Move, infos []replay.MoveInfo, startWhite, blackNeedsNumber bool, opts Options) {
	for i, mv := range moves {
		var info *replay.MoveInfo
		if i < len(infos) {
			info = &infos[i]
		}

		isWhite := isWhiteMove(startWhite, i)

		if !opts.NoMoveNumbers {
			if isWhite {
				lw.WriteToken(strconv.Itoa(moveNumberOf(moves, i, startWhite)) + ".")
			} else if blackNeedsNumber {
				lw.WriteToken(strconv.Itoa(moveNumberOf(moves, i, startWhite)) + "...")
			}
		}

		lw.WriteToken(sanFor(mv, info, opts))
		blackNeedsNumber = false

		if !opts.NoNAGs {
			for _, n := range mv.NAGs {
				lw.WriteToken("$" + strconv.Itoa(n))
			}
		}

		if !opts.NoComments {
			for _, c := range mv.Comments {
				emitComment(lw, c)
			}
			if len(mv.Comments) > 0 {
				blackNeedsNumber = true
			}
		}

		if !opts.NoVariations && len(mv.Variations) > 0 {
			for vi, v := range mv.Variations {
				var vInfos []replay.MoveInfo
				if info != nil && vi < len(info.Variations) && info.Variations[vi] != nil {
					vInfos = info.Variations[vi].Moves
				}
				// The variation is an alternative to mv, so it starts on
				// the same side to move as mv.
				emitVariation(lw, v, vInfos, isWhite, opts)
			}
			blackNeedsNumber = true
		}
	}
}

// emitVariation writes "(...)" for a single variation. The opening paren
// is glued (no space) directly to the variation's first move token when
// there are no prefix comments; when the variation opens with a comment,
// the C oracle inserts a space between "(" and the comment. The closing
// paren is always glued to the immediately preceding token.
func emitVariation(lw *lineWriter, v *pgn.Variation, infos []replay.MoveInfo, startWhite bool, opts Options) {
	lw.WriteToken("(")
	if !opts.NoComments && len(v.PrefixComments) > 0 {
		for _, c := range v.PrefixComments {
			emitComment(lw, c)
		}
	} else {
		lw.SuppressSpace()
	}
	// A variation always starts with an explicit move number: white moves
	// get "N.", and black moves (the variation's first move being black's)
	// get "N...".
	emitLine(lw, v.Moves, infos, startWhite, true, opts)
	lw.SuppressSpace()
	lw.WriteToken(")")
}

// emitComment writes a comment as "{", each whitespace-separated word of
// its text, and "}" — all as individual tokens, so long comments word-wrap
// at the configured line width exactly like the C oracle (which splits
// comment text into individually wrappable words). The single spaces
// inside the braces ("{ text }") fall out of normal token separation.
func emitComment(lw *lineWriter, c string) {
	lw.WriteToken("{")
	for _, word := range strings.Fields(c) {
		lw.WriteToken(word)
	}
	lw.WriteToken("}")
}

// sanFor returns the token to emit for mv, using replay's normalized SAN
// when opts.Format == SAN (and info is available), else the SAN exactly as
// written in the source.
func sanFor(mv *pgn.Move, info *replay.MoveInfo, opts Options) string {
	if opts.Format == SAN && info != nil {
		return info.SAN
	}
	return mv.SAN
}

// isWhiteMove reports whether moves[i] within a line that begins with
// startWhite to move is a white move.
func isWhiteMove(startWhite bool, i int) bool {
	if startWhite {
		return i%2 == 0
	}
	return i%2 == 1
}

// moveNumberOf returns the full-move number for moves[i] in a line that
// begins with startWhite to move, computed from the nearest preceding move
// that carries an explicit pgn.Move.MoveNumber (falling back to counting
// forward/backward across the ply sequence when none is set).
func moveNumberOf(moves []*pgn.Move, i int, startWhite bool) int {
	if moves[i].MoveNumber > 0 {
		return moves[i].MoveNumber
	}
	// Search backward for the closest move with an explicit number and
	// derive from ply distance.
	for j := i - 1; j >= 0; j-- {
		if moves[j].MoveNumber > 0 {
			return moves[j].MoveNumber + (i-j)/2
		}
	}
	// Search forward as a fallback.
	for j := i + 1; j < len(moves); j++ {
		if moves[j].MoveNumber > 0 {
			return moves[j].MoveNumber - (j-i)/2
		}
	}
	// No move in the line carries an explicit number at all: derive from
	// scratch, using ply 0's number as 1 (or, if the line starts on
	// black's move, black's first move is still full-move 1).
	if startWhite {
		return i/2 + 1
	}
	return (i+1)/2 + 1
}
