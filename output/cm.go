package output

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// The ChessMaster layout's column widths (output.c MOVE_NUMBER_WIDTH,
// MOVE_WIDTH, COMMENT_INDENT): a move pair is a right-aligned move
// number, ". ", and two left-aligned move columns; a comment starts in
// the column after them.
const (
	cmMoveNumberWidth = 3
	cmMoveWidth       = 15
	cmCommentIndent   = cmMoveNumberWidth + 2 + 2*cmMoveWidth
	cmCommentChar     = ';'
)

// writeCMGame renders g in ChessMaster format (output.c output_cm_game,
// "probably obsolete" by its own account): a "WHITE: name" and "BLACK:
// name" header from those two tags alone (empty when absent), a blank
// line, the prefix comments as one ";" comment, then the main line one
// move pair per line, every move in SAN with checkmate spelled "++"
// (apply.c rewrite_SAN_string under the CM format), and a newline to end
// the game. Comments follow the move they annotate as ";" comments, word
// wrapped at the line width and indented to the comment column; a comment
// after white's move pushes black's move to the next line behind a "..."
// placeholder, with "..." again in white's column. The game's result is
// a ";" comment in words ("and black resigns", "and white resigns",
// "draw", "incomplete result") after the last move, unless that move is
// checkmate. The format is the C tool's: a game that ends with white's
// mating move has no blank line after it, a game starting from a FEN is
// still numbered from that position (the C tool warns and writes it
// anyway), and under NoComments a game with a prefix comment still gets a
// bare ";" line, as -C leaves the C tool an empty comment to print.
//
// Variations and NAGs are never written, and the format ignores every
// other output option: NoChecks, NoResults, NoMoveNumbers, NoTags,
// CommentLines, the ply limit and the generated comments (FENComments,
// HashComments, Evaluation, FinalFEN), none of which reach output_cm_game.
// Only LineWidth (comment wrapping) and NoComments apply.
func (w *Writer) writeCMGame(g *pgn.Game, r *replay.Result, opts Options) error {
	width := opts.LineWidth
	if width <= 0 {
		width = defaultLineWidth
	}
	var b bytes.Buffer
	white, _ := g.Tag("White")
	black, _ := g.Tag("Black")
	fmt.Fprintf(&b, "WHITE: %s\nBLACK: %s\n\n", white, black)

	if len(g.PrefixComments) > 0 {
		comments := g.PrefixComments
		if opts.NoComments {
			comments = nil
		}
		cmComment(&b, comments, 0, width)
	}

	moveNumber, whiteToMove := mainLineStart(r)
	var infos []replay.MoveInfo
	if r != nil {
		infos = r.Moves
	}
	for i, mv := range g.Moves {
		var info *replay.MoveInfo
		if i < len(infos) {
			info = &infos[i]
		}
		if whiteToMove {
			fmt.Fprintf(&b, "%*d. %-*s", cmMoveNumberWidth, moveNumber, cmMoveWidth, cmMove(mv, info))
			whiteToMove = false
		} else {
			fmt.Fprintf(&b, "%-*s", cmMoveWidth, cmMove(mv, info))
			moveNumber++
			whiteToMove = true
		}

		// grammar.c attaches the result token to the last move as its
		// terminating_result; a game without one has none.
		result := ""
		if i == len(g.Moves)-1 {
			result = g.Result
		}
		mate := info != nil && info.IsMate
		// Only the move's comment_list is printed: the comments written
		// directly after it. Those after a NAG or a variation belong to
		// the NAG or the variation in the C grammar and never reach
		// output_cm_game.
		var comments []string
		if !opts.NoComments {
			comments = mv.LeadingComments()
		}

		switch {
		case len(comments) > 0:
			if !whiteToMove {
				// White's comment takes black's column; black's move
				// follows on the next line.
				cmPlaceholder(&b)
			}
			cmComment(&b, comments, cmCommentIndent, width)
			if result != "" && !mate {
				if whiteToMove {
					fmt.Fprintf(&b, "%*s", cmCommentIndent, "")
				} else {
					cmPlaceholderLine(&b)
					fmt.Fprintf(&b, "%*s", cmMoveWidth, "")
				}
				cmResult(&b, result)
				b.WriteByte('\n')
			} else if !whiteToMove {
				cmPlaceholderLine(&b)
			}
		case result != "" && !mate:
			if !whiteToMove {
				cmPlaceholder(&b)
			}
			cmResult(&b, result)
			if !whiteToMove {
				b.WriteByte('\n')
				cmPlaceholderLine(&b)
			}
			b.WriteByte('\n')
		case whiteToMove:
			// Black's move ends the pair's line.
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')
	_, err := w.w.Write(b.Bytes())
	return err
}

// cmPlaceholder writes "..." in a move column, standing in for the move
// of the side that has not moved yet on this line.
func cmPlaceholder(b *bytes.Buffer) {
	fmt.Fprintf(b, "%-*s", cmMoveWidth, "...")
}

// cmPlaceholderLine writes the start of a line whose white column is a
// placeholder: a blank move number and "..." where white's move would be.
func cmPlaceholderLine(b *bytes.Buffer) {
	fmt.Fprintf(b, "%*s ", cmMoveNumberWidth+1, "")
	cmPlaceholder(b)
}

// CMFromStart reports whether a game's replay starts where the CM format
// expects, white to move at move 1: output.c output_cm_game logs "Unable
// to output CM games other than from the starting position." otherwise
// and writes the game anyway, as writeCMGame does, so the caller decides
// whether to say so. A game without a replay is taken as starting there.
func CMFromStart(r *replay.Result) bool {
	moveNumber, whiteToMove := mainLineStart(r)
	return moveNumber == 1 && whiteToMove
}

// cmMove is the move as the CM format spells it: replay's SAN with a
// checkmate marked "++" rather than "#", and the English piece letters
// whatever PieceLetters says (output.c set_output_format reads the -W
// suffix for san, elalg and xlalg only). Without replay detail the move
// is written as the source spelled it.
func cmMove(mv *pgn.Move, info *replay.MoveInfo) string {
	if info == nil {
		return mv.SAN
	}
	if info.IsMate {
		return strings.TrimSuffix(info.SAN, "#") + "++"
	}
	return info.SAN
}

// cmComment writes comments as one ";" comment (output.c
// output_cm_comment): ";" at the current position, column indent, then
// every word of every comment preceded by a space, moving to a new line
// indented by indent and starting "; " when the word would take the line
// past width; the comment ends its line. The words are what strtok(" ")
// finds on each source line of the comment, so runs of spaces and line
// breaks separate words and other whitespace stays inside them.
func cmComment(b *bytes.Buffer, comments []string, indent, width int) {
	b.WriteByte(cmCommentChar)
	lineLen := indent + 1
	for _, c := range comments {
		for _, word := range cmWords(c) {
			if lineLen+1+len(word) > width {
				b.WriteByte('\n')
				for i := 0; i < indent; i++ {
					b.WriteByte(' ')
				}
				b.WriteByte(cmCommentChar)
				b.WriteByte(' ')
				lineLen = indent + 2
			} else {
				b.WriteByte(' ')
				lineLen++
			}
			b.WriteString(word)
			lineLen += len(word)
		}
	}
	b.WriteByte('\n')
}

// cmWords splits a comment into the words the C tool prints: its lexer
// stores a comment one source line at a time (CR or LF ending a line)
// and output_cm_comment breaks each line on spaces alone.
func cmWords(c string) []string {
	return strings.FieldsFunc(c, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\r'
	})
}

// cmResult writes the result as the words of a ";" comment (output.c
// output_cm_result), without a newline.
func cmResult(b *bytes.Buffer, result string) {
	b.WriteByte(cmCommentChar)
	b.WriteByte(' ')
	switch {
	case result == "1-0":
		b.WriteString("and black resigns")
	case result == "0-1":
		b.WriteString("and white resigns")
	case strings.HasPrefix(result, "1/2"):
		b.WriteString("draw")
	default:
		b.WriteString("incomplete result")
	}
}
