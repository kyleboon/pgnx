package output

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/kyleboon/pgnx/chess"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// errJSONFormat is returned when JSON output is asked for in a format
// that has no JSON form; the C tool turns --json off with a warning for
// -Wepd and -Wcm (main.c), pgnx refuses.
var errJSONFormat = errors.New("output: JSON output is not available in epd, fen or cm format")

// HasJSON reports whether f has a JSON form (Options.JSON, --json): every
// movetext format does; EPD, FEN and CM do not (main.c: "JSON output is
// not currently supported with -E, -Wepd or -Wcm").
func (f Format) HasJSON() bool {
	return f != EPD && f != FEN && f != CM
}

// writeJSONGame renders g as one object of the JSON array the Writer's
// games form (output.c print_algebraic_game under json_format): "[" or
// the "," that separates it from the previous game, then the tags as
// members each on a line with a trailing comma, a blank line, and the
// "Moves" array, laid out exactly as the C tool lays it out. opts is
// w.opts with the format WriteGame settled on, which spells the moves.
func (w *Writer) writeJSONGame(g *pgn.Game, r *replay.Result, opts Options) error {
	if !opts.Format.HasJSON() {
		return errJSONFormat
	}
	sep := ",\n"
	if w.games == 0 {
		sep = "[\n"
	}
	if _, err := io.WriteString(w.w, sep+"{\n"); err != nil {
		return err
	}
	w.games++
	if !opts.NoTags {
		if err := w.writeTags(g); err != nil {
			return err
		}
		if _, err := io.WriteString(w.w, "\n"); err != nil {
			return err
		}
	}
	var b strings.Builder
	b.WriteString(`"Moves":[`)
	jsonMoves(&b, g, r, opts)
	b.WriteString("]\n\n}")
	_, err := io.WriteString(w.w, b.String())
	return err
}

// Close ends the JSON array the Writer's games form, when there is one
// to end: the C tool writes nothing at all when no game was written, and
// closes an array only after its last game. It is a no-op for every
// other output and after it has ended the array, so a caller may always
// call it, and more than once, when the games are written.
func (w *Writer) Close() error {
	if !w.opts.JSON || w.games == 0 {
		return nil
	}
	w.games = 0
	_, err := io.WriteString(w.w, "\n]\n")
	return err
}

// jsonMoves writes the elements of a game's "Moves" array: one object
// per main-line move, "{ "move" : "..." }" with the move's NAGs and the
// generated fields between (output.c print_move_list, print_move and
// print_items_following_move under json_format), the objects separated
// by ", ". --ply-limit cuts the array short with no result marker, since
// JSON output carries no result token.
func jsonMoves(b *strings.Builder, g *pgn.Game, r *replay.Result, opts Options) {
	baseMoveNumber, startWhite := mainLineStart(r)
	startPly := firstPly(baseMoveNumber, startWhite)
	var infos []replay.MoveInfo
	if r != nil {
		infos = r.Moves
	}
	var board *chess.Board
	if opts.Evaluation && r != nil {
		board = r.StartBoard()
	}
	noNAGs := opts.NoNAGs || isUCI(opts.Format)

	for i, mv := range g.Moves {
		if opts.HasPlyLimit && startPly+i > opts.PlyLimit {
			return
		}
		var info *replay.MoveInfo
		if i < len(infos) {
			info = &infos[i]
		}
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(`{ "move" : "`)
		b.WriteString(renderMove(mv, info, opts))
		b.WriteString(`"`)

		if !noNAGs {
			// The C tool writes the NAG list between "[" and "] " and
			// separates the NAGs with ", " (it leaves the comma out
			// between NAGs that follow one another directly; pgnx does
			// not reproduce that, see CONFORMANCE.md).
			var nags []string
			for _, a := range mv.Annotations {
				if n, ok := a.(pgn.NAG); ok {
					nags = append(nags, `"$`+strconv.Itoa(int(n))+`"`)
				}
			}
			if len(nags) > 0 {
				b.WriteString(`, "nags" : [`)
				b.WriteString(strings.Join(nags, ", "))
				b.WriteString("] ")
			}
		}
		if board != nil && info != nil {
			board.Apply(info.Move)
			fmt.Fprintf(b, `, "evaluation" : "%.2f"`, shannonEvaluation(board))
		}
		if r != nil && i+1 < len(r.Positions) {
			if opts.FENComments {
				fmt.Fprintf(b, `, "FEN" : "%s"`, r.Positions[i+1].FEN)
			}
			if opts.HashComments {
				fmt.Fprintf(b, `, "HashCode" : "%016x"`, r.Positions[i+1].Zobrist)
			}
		}
		if i == len(g.Moves)-1 && opts.FinalFEN && !opts.FENComments && r != nil && len(r.Positions) > len(g.Moves) {
			// -F: the final position follows the last move's own fields,
			// with the trailing space the C tool writes after it
			// (print_move_list).
			fmt.Fprintf(b, `, "FEN" : "%s" `, r.Positions[len(g.Moves)].FEN)
		}
		b.WriteString(" }")
	}
}
