package output

import (
	"fmt"
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
// any other Options. The generated comments (--fencomments and friends)
// are not among the things it disables.
func isUCI(f Format) bool { return f == UCI }

// emitter renders one game's movetext, token by token, into a lineWriter
// under a fixed set of Options. It is what output.c print_move_list,
// print_move and print_items_following_move are to the C tool.
type emitter struct {
	lw   *lineWriter
	opts Options
}

// movetext writes g's movetext (main line, recursively including
// variations), the final-position comment if asked for, and the result
// token (unless opts.NoResults).
func (e *emitter) movetext(g *pgn.Game, r *replay.Result) {
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
	cut := e.line(g.Moves, r, startWhite, true, baseMoveNumber)

	if cut {
		// print_move_list: a main line cut short by --plylimit ends
		// with "*" when the game had a result, whatever it was.
		if !e.opts.NoResults && g.Result != "" {
			e.lw.WriteToken("*")
		}
		return
	}

	if e.opts.FinalFEN && !e.opts.FENComments && r != nil && len(g.Moves) > 0 && len(r.Positions) > len(g.Moves) {
		// -F: the final position, quoted, after the last move's own
		// annotations and before the result; not subject to
		// --nocomments (output.c print_move_list).
		e.comment(`"` + r.Positions[len(g.Moves)].FEN + `"`)
	}

	if !e.opts.NoResults {
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
		e.lw.WriteToken(result)
	}
}

// line writes one line of movetext (a main line or the body of a
// variation): moves is the pgn.Move sequence, r its replay (nil when
// replay data is unavailable, which is only valid for Format == Source).
// startWhite indicates whether moves[0] (if any) is white's move — true
// for the main line, and for a variation it matches the color of the
// move the variation is attached to (a RAV is an alternative to a single
// ply, so it begins on that same side to move). blackNeedsNumber tracks
// whether the very next move, if it is black's, must be given an
// explicit "N..." number because of a preceding interruption
// (comment/variation) or because this line starts with black to move.
// baseMoveNumber is the full-move number that ply 0 of moves belongs to
// when no move carries an explicit number (normally 1, or the starting
// position's FEN fullmove field for a game that begins mid-sequence);
// see movetext. It reports whether --plylimit cut the line short.
func (e *emitter) line(moves []*pgn.Move, r *replay.Result, startWhite, blackNeedsNumber bool, baseMoveNumber int) (cut bool) {
	opts := e.opts
	uci := isUCI(opts.Format)
	noMoveNumbers := opts.NoMoveNumbers || uci
	noNAGs := opts.NoNAGs || uci
	noComments := opts.NoComments || uci
	noVariations := opts.NoVariations || uci

	var infos []replay.MoveInfo
	if r != nil {
		infos = r.Moves
	}
	// The evaluation needs the board after each move; walk it alongside
	// the typed moves.
	var board *chess.Board
	if opts.Evaluation && r != nil {
		board = r.StartBoard()
	}
	// The ply number of moves[0], as print_move_list computes it from
	// the move number and side to move.
	startPly := 2*(baseMoveNumber-1) + 1
	if !startWhite {
		startPly++
	}

	for i, mv := range moves {
		if opts.HasPlyLimit && startPly+i > opts.PlyLimit {
			return true
		}
		var info *replay.MoveInfo
		if i < len(infos) {
			info = &infos[i]
		}

		isWhite := isWhiteMove(startWhite, i)

		if !noMoveNumbers {
			if isWhite {
				e.lw.WriteToken(strconv.Itoa(moveNumberOf(moves, i, startWhite, baseMoveNumber)) + ".")
			} else if blackNeedsNumber {
				e.lw.WriteToken(strconv.Itoa(moveNumberOf(moves, i, startWhite, baseMoveNumber)) + "...")
			}
		}

		e.lw.WriteToken(renderMove(mv, info, opts))
		blackNeedsNumber = false

		// print_items_following_move: the move's own annotations in the
		// order they were written, with the generated comments
		// (evaluation, FEN, hash) placed after the last NAG or comment
		// that precedes the first variation, as the C tool emits them
		// between its NAG list and its variations. A comment or a
		// variation is an interruption that makes black's next move
		// carry its number; a NAG is not.
		generated := false
		vi := 0
		for _, a := range mv.Annotations {
			switch a := a.(type) {
			case pgn.NAG:
				if !noNAGs {
					e.lw.WriteToken("$" + strconv.Itoa(int(a)))
				}
			case pgn.Comment:
				if !noComments {
					e.comment(string(a))
					blackNeedsNumber = true
				}
			case *pgn.Variation:
				if !generated {
					generated = true
					if e.generatedComments(board, info, r, i) {
						blackNeedsNumber = true
					}
				}
				if !noVariations {
					// The variation is an alternative to mv, so it
					// starts on the same side to move, and the same
					// full-move number, as mv.
					var vr *replay.Result
					if info != nil && vi < len(info.Variations) {
						vr = info.Variations[vi]
					}
					e.variation(a, vr, isWhite, moveNumberOf(moves, i, startWhite, baseMoveNumber))
					blackNeedsNumber = true
				}
				vi++
			}
		}
		if !generated && e.generatedComments(board, info, r, i) {
			blackNeedsNumber = true
		}
	}
	return false
}

// generatedComments writes the comments the run asked pgnx to generate
// after main-line move i (--evaluation, --fen-comments, --hash-comments),
// in that order, advancing board for the evaluation; it reports whether
// it wrote any.
func (e *emitter) generatedComments(board *chess.Board, info *replay.MoveInfo, r *replay.Result, i int) (wrote bool) {
	if board != nil && info != nil {
		board.Apply(info.Move)
		e.comment(fmt.Sprintf("%.2f", shannonEvaluation(board)))
		wrote = true
	}
	if r != nil && i+1 < len(r.Positions) {
		if e.opts.FENComments {
			e.comment(r.Positions[i+1].FEN)
			wrote = true
		}
		if e.opts.HashComments {
			e.comment(fmt.Sprintf("%016x", r.Positions[i+1].Zobrist))
			wrote = true
		}
	}
	return wrote
}

// variation writes "(...)" for a single variation. The opening paren
// is glued (no space) directly to the variation's first move token when
// there are no prefix comments; when the variation opens with a comment,
// the C oracle inserts a space between "(" and the comment. The closing
// paren is always glued to the immediately preceding token.
func (e *emitter) variation(v *pgn.Variation, r *replay.Result, startWhite bool, baseMoveNumber int) {
	e.lw.WriteToken("(")
	if !e.opts.NoComments && len(v.PrefixComments) > 0 {
		for _, c := range v.PrefixComments {
			e.comment(c)
		}
	} else {
		e.lw.SuppressSpace()
	}
	// A variation always starts with an explicit move number: white moves
	// get "N.", and black moves (the variation's first move being black's)
	// get "N...".
	e.line(v.Moves, r, startWhite, true, baseMoveNumber)
	if v.Result != "" && !e.opts.NoResults {
		// A result written inside the variation follows its last move
		// (output.c print_move_list prints a move's terminating_result).
		e.lw.WriteToken(v.Result)
	}
	e.lw.SuppressSpace()
	e.lw.WriteToken(")")
}

// comment writes a comment as "{", each whitespace-separated word of
// its text, and "}" — all as individual tokens, so long comments word-wrap
// at the configured line width exactly like the C oracle (which splits
// comment text into individually wrappable words). The single spaces
// inside the braces ("{ text }") fall out of normal token separation.
// With CommentLines the comment is a line of its own (output.c
// start_comment/end_comment).
func (e *emitter) comment(c string) {
	if e.opts.CommentLines {
		e.lw.BreakLine()
	}
	e.lw.WriteToken("{")
	for _, word := range strings.Fields(c) {
		e.lw.WriteToken(word)
	}
	e.lw.WriteToken("}")
	if e.opts.CommentLines {
		e.lw.BreakLine()
	}
}

// renderMove returns the token to emit for mv in the format selected by
// opts.Format. info carries the replayed detail (piece, capture, check)
// needed by every format except Source. For SAN it's replay's normalized
// SAN; for Source (or any format when info is nil) it's the SAN exactly as
// written in the source. opts.NoChecks (and UCI) drop the check suffix.
func renderMove(mv *pgn.Move, info *replay.MoveInfo, opts Options) string {
	noChecks := opts.NoChecks || isUCI(opts.Format)
	switch opts.Format {
	case LAlg, HAlg, ELAlg, XLAlg, UCI:
		if info != nil {
			return renderAlgebraic(*info, opts.Format, noChecks)
		}
	case SAN:
		if info != nil {
			return stripChecks(info.SAN, noChecks)
		}
	}
	return stripChecks(mv.SAN, noChecks)
}

// stripChecks cuts san at its first "+" (or, failing that, "#") when
// noChecks, as output.c print_move does for SAN and Source: whatever
// follows the glyph goes with it.
func stripChecks(san string, noChecks bool) string {
	if !noChecks {
		return san
	}
	i := strings.IndexByte(san, '+')
	if i < 0 {
		i = strings.IndexByte(san, '#')
	}
	if i < 0 {
		return san
	}
	return san[:i]
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
//     in output.c, which only appends "ep" for ELALG/XLALG/XOLALG), and
//     promotions are still the uppercase piece_str() letter.
//
// Castling is rendered as the king's own from-to move (never "O-O"), with
// no piece-letter prefix (castling is not class PIECE_MOVE in the C
// source) and, for HAlg/XLAlg, a "-" separator (castling is never a
// capture). A null move is "--" in every one of these formats, UCI
// included (print_move's NULL_MOVE case writes NULL_MOVE_STRING; the C
// tool never writes "0000"). The check suffix is appended unless
// noChecks.
func renderAlgebraic(mi replay.MoveInfo, f Format, noChecks bool) string {
	m := mi.Move
	var b strings.Builder
	if m.IsNull() {
		return replay.NullMoveSAN + checkSuffix(mi, noChecks)
	}

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

	b.WriteString(checkSuffix(mi, noChecks))
	return b.String()
}

// checkSuffix is the "+"/"#" suffix mi gets in the long-algebraic family,
// or "" when noChecks.
func checkSuffix(mi replay.MoveInfo, noChecks bool) string {
	switch {
	case noChecks:
		return ""
	case mi.IsMate:
		return "#"
	case mi.IsCheck:
		return "+"
	}
	return ""
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
