package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// writePositionGame renders g in EPD or FEN format: one line per position
// of the main line (the initial position plus one per main-line move;
// unlike movetext rendering, variations are not visited, mirroring the C
// oracle's print_EPD_move_list/print_FEN_move_list, which walk only
// current_game->moves). The FEN of each position is built on the way by
// walking r's typed moves from its start board, as the C tool builds FEN
// components only for these formats (apply.c). Neither format uses
// lineWriter wrapping or the Options.No* move-text flags — the C oracle's
// print_EPD_game/print_FEN_game call fprintf directly, with no equivalent
// of print_move_list's stripping flags.
func (w *Writer) writePositionGame(g *pgn.Game, r *replay.Result) error {
	switch w.opts.Format {
	case EPD:
		return w.writeEPDGame(g, r)
	case FEN:
		return w.writeFENGame(g, r)
	default:
		return fmt.Errorf("output: writePositionGame called for non-position format %v", w.opts.Format)
	}
}

// writeEPDGame renders g as EPD: no tag section (output.c:print_EPD_game
// only ever reaches this format with GlobalState.tag_output_format ==
// NO_TAGS in practice), one "<epd> c0 <comment>;" line per position, and a
// trailing blank line.
func (w *Writer) writeEPDGame(g *pgn.Game, r *replay.Result) error {
	// --notags leaves the comment empty, trailing space and all
	// (print_EPD_move_list's "%s %s" with an empty game comment).
	comment := ""
	if !w.opts.NoTags {
		comment = epdGameComment(g)
	}
	err := walkFENs(r, func(fen string) error {
		_, err := fmt.Fprintf(w.w, "%s %s\n", epdFields(fen), comment)
		return err
	})
	if err != nil {
		return err
	}
	_, err = io.WriteString(w.w, "\n")
	return err
}

// walkFENs calls fn with the FEN of every position of r's line in order,
// the start position first, stopping at fn's first error. A nil or empty
// r has no positions.
func walkFENs(r *replay.Result, fn func(fen string) error) error {
	b := r.StartBoard()
	if b == nil {
		return nil
	}
	if err := fn(b.FEN()); err != nil {
		return err
	}
	for i := range r.Moves {
		b.Apply(r.Moves[i].Move)
		if err := fn(b.FEN()); err != nil {
			return err
		}
	}
	return nil
}

// writeFENGame renders g as FEN: the same tag section as SAN/Source
// (output.c:print_FEN_game's default tag_output_format is ALL_TAGS, calling
// show_tags exactly as print_algebraic_game does), a blank line, one full
// 6-field FEN string per position, and a trailing blank line. There is no
// result token and no per-position comment (unlike EPD).
func (w *Writer) writeFENGame(g *pgn.Game, r *replay.Result) error {
	if !w.opts.NoTags {
		if err := w.writeTags(g); err != nil {
			return err
		}
		if _, err := io.WriteString(w.w, "\n"); err != nil {
			return err
		}
	}
	// print_FEN_move_list tests --plylimit once, against the starting
	// ply: a start past the limit lists nothing, a start within it lists
	// every position.
	if !(w.opts.HasPlyLimit && len(r.Positions) > 0 && r.Positions[0].PlyNumber() > w.opts.PlyLimit) {
		err := walkFENs(r, func(fen string) error {
			_, err := fmt.Fprintf(w.w, "%s\n", fen)
			return err
		})
		if err != nil {
			return err
		}
	}
	_, err := io.WriteString(w.w, "\n")
	return err
}

// epdFields returns the EPD (4-field) prefix of a full 6-field FEN string:
// piece placement, side to move, castling availability, and en passant
// target square — dropping the halfmove clock and fullmove number, per
// apply.c:build_basic_EPD_string (which the C oracle shares between EPD and
// the first four fields of FEN).
func epdFields(fen string) string {
	fields := strings.Fields(fen)
	if len(fields) < 4 {
		return fen
	}
	return strings.Join(fields[:4], " ")
}

// epdGameComment builds the "c0 <White>-<Black> <Event> <Site> <Date>; c1
// <Result>;" comment appended to every EPD line, mirroring
// output.c:format_epd_game_comment byte for byte: White/Black are joined
// with a bare "-" (present even when both are absent); Event/Site/Date are
// each written only when present, prefixed with a single space; Result
// defaults to "*" when the tag is absent.
func epdGameComment(g *pgn.Game) string {
	var b strings.Builder
	b.WriteString("c0 ")
	if white, ok := g.Tag("White"); ok {
		b.WriteString(white)
	}
	b.WriteByte('-')
	if black, ok := g.Tag("Black"); ok {
		b.WriteString(black)
	}
	for _, name := range []string{"Event", "Site", "Date"} {
		if value, ok := g.Tag(name); ok {
			b.WriteByte(' ')
			b.WriteString(value)
		}
	}
	b.WriteString("; c1 ")
	if result, ok := g.Tag("Result"); ok && result != "" {
		b.WriteString(result)
	} else {
		b.WriteByte('*')
	}
	b.WriteByte(';')
	return b.String()
}
