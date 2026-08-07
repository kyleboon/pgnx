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
	var positions []replay.Position
	if r != nil {
		moveInfos = r.Moves
		positions = r.Positions
	}
	// The main line always starts with white to move.
	emitLine(lw, g.Moves, moveInfos, positions, true, true, opts)

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
// which is only valid for Format == Source). positions holds the position
// before each move in moves (positions[i] is the position moves[i] is
// played from), used only to detect en passant captures for the ELAlg/XLAlg
// "ep" suffix; it may be shorter than moves or nil when unavailable.
// startWhite indicates whether moves[0] (if any) is white's move — true for
// the main line, and for a variation it matches the color of the move the
// variation is attached to (a RAV is an alternative to a single ply, so it
// begins on that same side to move). blackNeedsNumber tracks whether the
// very next move, if it is black's, must be given an explicit "N..." number
// because of a preceding interruption (comment/variation) or because this
// line starts with black to move.
func emitLine(lw *lineWriter, moves []*pgn.Move, infos []replay.MoveInfo, positions []replay.Position, startWhite, blackNeedsNumber bool, opts Options) {
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
		var before *replay.Position
		if i < len(positions) {
			before = &positions[i]
		}

		isWhite := isWhiteMove(startWhite, i)

		if !noMoveNumbers {
			if isWhite {
				lw.WriteToken(strconv.Itoa(moveNumberOf(moves, i, startWhite)) + ".")
			} else if blackNeedsNumber {
				lw.WriteToken(strconv.Itoa(moveNumberOf(moves, i, startWhite)) + "...")
			}
		}

		lw.WriteToken(renderMove(mv, info, before, opts))
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
			for vi, v := range mv.Variations {
				var vInfos []replay.MoveInfo
				var vPositions []replay.Position
				if info != nil && vi < len(info.Variations) && info.Variations[vi] != nil {
					vInfos = info.Variations[vi].Moves
					vPositions = info.Variations[vi].Positions
				}
				// The variation is an alternative to mv, so it starts on
				// the same side to move as mv.
				emitVariation(lw, v, vInfos, vPositions, isWhite, opts)
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
func emitVariation(lw *lineWriter, v *pgn.Variation, infos []replay.MoveInfo, positions []replay.Position, startWhite bool, opts Options) {
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
	emitLine(lw, v.Moves, infos, positions, startWhite, true, opts)
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
// needed by every format except Source; before is the position mv is
// played from (used only to detect en passant captures for the ELAlg/XLAlg
// "ep" suffix). For SAN it's replay's normalized SAN; for Source (or any
// format when info is nil) it's the SAN exactly as written in the source.
func renderMove(mv *pgn.Move, info *replay.MoveInfo, before *replay.Position, opts Options) string {
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
		return renderAlgebraic(*info, before, opts.Format)
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
func renderAlgebraic(mi replay.MoveInfo, before *replay.Position, f Format) string {
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
	} else if (f == ELAlg || f == XLAlg) && isEnPassant(mi, before) {
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

// isEnPassant reports whether mi is an en passant capture: a pawn move that
// captures a pawn not standing on the destination square. before (the
// position mi.Move is played from) is consulted because MoveInfo alone
// can't distinguish this from an ordinary pawn-takes-pawn capture — the
// signal is that before's en passant target square (its FEN's 4th field)
// equals the move's destination.
func isEnPassant(mi replay.MoveInfo, before *replay.Position) bool {
	if mi.Piece.Type != chess.Pawn || mi.Captured != chess.Pawn || before == nil {
		return false
	}
	fields := strings.Fields(before.FEN)
	if len(fields) < 4 {
		return false
	}
	return fields[3] == mi.Move.To.String()
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
