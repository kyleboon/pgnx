package output

import (
	"strconv"
	"strings"

	"github.com/kyleboon/pgnx/chess"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// isUCI reports whether f is the UCI format, which the C oracle renders by
// forcibly disabling move numbers, comments, NAGs, variations, and check
// suffixes (argsfile.c: selecting -Wuci sets keep_NAGs/keep_comments/
// keep_move_numbers/keep_checks/keep_variations all FALSE), regardless of
// any other Options. Since Options has no NoChecks field (and none is
// needed for any other format), UCI's stripping is special-cased here
// rather than exposed as new Options surface.
func isUCI(f Format) bool { return f == UCI }

// emitMovetext writes g's movetext (main line, recursively including
// variations) followed by the result token (unless opts.NoResults) to lw.
func emitMovetext(lw *lineWriter, g *pgn.Game, r *replay.Result, opts Options) {
	var moveInfos []replay.MoveInfo
	if r != nil {
		moveInfos = r.Moves
	}
	// Source move numbers are ignored: the C oracle numbers every move
	// from its own counter, seeded from the starting position's fullmove
	// number and side to move (output.c print_game: "move_number =
	// initial_board->move_number"; format_game: white_to_move =
	// initial_board->to_move == WHITE). So a game starting from a FEN
	// with a nonzero fullmove count (an endgame study "[FEN \"... w - - 6
	// 55\"]") numbers its first move "55.", and a black-to-move start is
	// numbered "N..." with the next move N+1. Without a replay (Source
	// format) the standard start is assumed.
	baseMoveNumber, startWhite := 1, true
	if r != nil && len(r.Positions) > 0 {
		baseMoveNumber = r.Positions[0].FullmoveNumber
		startWhite = r.Positions[0].SideToMove == chess.White
	}
	emitLine(lw, g.Moves, moveInfos, startWhite, true, baseMoveNumber, opts)

	if !opts.NoResults {
		result := g.Result
		if len(g.Moves) == 0 {
			// A zero-move game has no move to carry a terminating result,
			// so the C oracle prints the Result tag instead (output.c:
			// "Take account of a possible zero move game"), whatever
			// result token the movetext held. This is what a game whose
			// movetext was only a stray token ("+/-", "N") gets.
			if value, ok := g.Tag("Result"); ok {
				result = value
			}
		}
		if result == "" {
			result = "*"
		}
		lw.WriteToken(result)
	}
}

// emitLine writes one line of movetext (a main line or the body of a
// variation): moves is the pgn.Move sequence, infos is the parallel
// replay.MoveInfo sequence (nil/empty when replay data is unavailable,
// which is only valid for Format == Source). startWhite indicates whether moves[0] (if any) is white's move — true for
// the main line, and for a variation it matches the color of the move the
// variation is attached to (a RAV is an alternative to a single ply, so it
// begins on that same side to move). blackNeedsNumber tracks whether the
// very next move, if it is black's, must be given an explicit "N..." number
// because of a preceding interruption (comment/variation) or because this
// line starts with black to move. baseMoveNumber is the full-move number
// that ply 0 of moves belongs to when no move carries an explicit number
// (normally 1, or the starting position's FEN fullmove field for a game
// that begins mid-sequence); see emitMovetext.
func emitLine(lw *lineWriter, moves []*pgn.Move, infos []replay.MoveInfo, startWhite, blackNeedsNumber bool, baseMoveNumber int, opts Options) {
	uci := isUCI(opts.Format)
	noMoveNumbers := opts.NoMoveNumbers || uci
	noNAGs := opts.NoNAGs || uci
	noComments := opts.NoComments || uci
	noVariations := opts.NoVariations || uci

	for i, mv := range moves {
		var info *replay.MoveInfo
		if i < len(infos) {
			info = &infos[i]
		}

		isWhite := isWhiteMove(startWhite, i)

		if !noMoveNumbers {
			if isWhite {
				lw.WriteToken(strconv.Itoa(moveNumberOf(moves, i, startWhite, baseMoveNumber)) + ".")
			} else if blackNeedsNumber {
				lw.WriteToken(strconv.Itoa(moveNumberOf(moves, i, startWhite, baseMoveNumber)) + "...")
			}
		}

		lw.WriteToken(renderMove(mv, info, opts))
		blackNeedsNumber = false

		if !noNAGs {
			for _, n := range mv.NAGs {
				lw.WriteToken("$" + strconv.Itoa(n))
			}
		}

		if !noComments {
			for _, c := range mv.Comments {
				emitComment(lw, c)
			}
			if len(mv.Comments) > 0 {
				blackNeedsNumber = true
			}
		}

		if !noVariations && len(mv.Variations) > 0 {
			// The variation is an alternative to mv, so it starts on the
			// same side to move, and the same full-move number, as mv.
			vBaseMoveNumber := moveNumberOf(moves, i, startWhite, baseMoveNumber)
			for vi, v := range mv.Variations {
				var vInfos []replay.MoveInfo
				if info != nil && vi < len(info.Variations) && info.Variations[vi] != nil {
					vInfos = info.Variations[vi].Moves
				}
				emitVariation(lw, v, vInfos, isWhite, vBaseMoveNumber, opts)
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
func emitVariation(lw *lineWriter, v *pgn.Variation, infos []replay.MoveInfo, startWhite bool, baseMoveNumber int, opts Options) {
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
	emitLine(lw, v.Moves, infos, startWhite, true, baseMoveNumber, opts)
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

// renderMove returns the token to emit for mv in the format selected by
// opts.Format. info carries the replayed detail (piece, capture, check)
// needed by every format except Source. For SAN it's replay's normalized
// SAN; for Source (or any format when info is nil) it's the SAN exactly as
// written in the source.
func renderMove(mv *pgn.Move, info *replay.MoveInfo, opts Options) string {
	switch opts.Format {
	case SAN:
		if info != nil {
			return info.SAN
		}
		return mv.SAN
	case Source:
		return mv.SAN
	case LAlg, HAlg, ELAlg, XLAlg, UCI:
		if info == nil {
			return mv.SAN
		}
		return renderAlgebraic(*info, opts.Format)
	default:
		return mv.SAN
	}
}

// pieceLetters is the C oracle's default (English) output_piece_characters,
// used to prefix ELAlg/XLAlg piece moves and to render promotions in every
// long-algebraic-family format (including UCI): promotion letters are
// uppercase, matching piece_str() in apply.c — not the lowercase convention
// used by the standalone UCI protocol / chess.Move.UCI().
var pieceLetters = map[chess.PieceType]string{
	chess.King:   "K",
	chess.Queen:  "Q",
	chess.Rook:   "R",
	chess.Knight: "N",
	chess.Bishop: "B",
}

// renderAlgebraic renders mi in one of the long-algebraic family formats
// (LAlg, HAlg, ELAlg, XLAlg, UCI), mirroring output.c:print_move's shared
// case for these formats:
//
//   - LAlg:  from+to, no separator, e.g. "e2e4".
//   - HAlg:  from+to with a "-" separator, even for captures, e.g. "e5-d4".
//   - ELAlg: piece-letter prefix for non-pawn, non-castling moves, then
//     from+to (no separator), e.g. "Ng1f3"; en passant gets an "ep" suffix.
//   - XLAlg: like ELAlg, but with a "-" or "x" separator between from and
//     to depending on whether the move captures; en passant gets "ep".
//   - UCI:   identical token shape to LAlg (no separator, no piece prefix,
//     no "ep" suffix — see UCI's UNKNOWN_MOVE/ENPASSANT_PAWN_MOVE handling
//     in output.c, which only appends "ep" for ELALG/XLALG/XOLALG); check
//     suffixes are never appended (argsfile.c forces keep_checks off), and
//     promotions are still the uppercase piece_str() letter.
//
// Castling is rendered as the king's own from-to move (never "O-O"), with
// no piece-letter prefix (castling is not class PIECE_MOVE in the C
// source) and, for HAlg/XLAlg, a "-" separator (castling is never a
// capture).
func renderAlgebraic(mi replay.MoveInfo, f Format) string {
	m := mi.Move
	var b strings.Builder

	// The C oracle only prefixes moves of class PIECE_MOVE — castling
	// (its own move class) and pawn moves never get a piece letter, but an
	// ordinary (non-castling) king move does.
	isPieceMove := mi.Piece.Type != chess.Pawn && m.Castle == chess.NoCastle
	if (f == ELAlg || f == XLAlg) && isPieceMove {
		b.WriteString(pieceLetters[mi.Piece.Type])
	}

	b.WriteString(m.From.String())

	switch f {
	case HAlg:
		// Hyphen even for captures — HAlg never uses "x".
		b.WriteByte('-')
	case XLAlg:
		if mi.Captured != chess.NoPiece {
			b.WriteByte('x')
		} else {
			b.WriteByte('-')
		}
	}

	b.WriteString(m.To.String())

	if m.Promotion != chess.NoPiece {
		b.WriteString(pieceLetters[m.Promotion])
	} else if (f == ELAlg || f == XLAlg) && mi.EnPassant {
		b.WriteString("ep")
	}

	if f != UCI {
		if mi.IsMate {
			b.WriteByte('#')
		} else if mi.IsCheck {
			b.WriteByte('+')
		}
	}

	return b.String()
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
// begins with startWhite to move, counting plies from baseMoveNumber (ply
// 0's full-move number).
func moveNumberOf(moves []*pgn.Move, i int, startWhite bool, baseMoveNumber int) int {
	// Source move numbers are deliberately ignored: the C oracle numbers
	// every move from its own counter (output.c print_move_list), seeded
	// from the starting position's fullmove number, so a source that
	// numbers a FEN game from 1 is still emitted with the FEN's numbers.
	// If the line starts on black's move, black's first move is still
	// full-move baseMoveNumber.
	if startWhite {
		return i/2 + baseMoveNumber
	}
	return (i+1)/2 + baseMoveNumber
}
