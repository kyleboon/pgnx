// Package output renders parsed PGN games (pgn.Game, optionally paired with
// replay.Result) back out in various textual formats.
package output

import (
	"fmt"
	"io"

	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// Format selects the output representation for a game's movetext.
type Format int

const (
	SAN    Format = iota // standard algebraic notation, normalized
	Source               // SAN exactly as it appeared in the source
	LAlg                 // long algebraic, e.g. e2e4
	HAlg                 // hyphenated long algebraic, e.g. e2-e4
	ELAlg                // long algebraic with piece letter, e.g. Ng1f3
	XLAlg                // long algebraic with piece letter and capture marker, e.g. Ng1xf3
	UCI                  // UCI move format
	EPD                  // Extended Position Description
	FEN                  // Forsyth-Edwards Notation
)

// String returns the canonical lowercase name for f, as accepted by
// ParseFormat.
func (f Format) String() string {
	switch f {
	case SAN:
		return "san"
	case Source:
		return "source"
	case LAlg:
		return "lalg"
	case HAlg:
		return "halg"
	case ELAlg:
		return "elalg"
	case XLAlg:
		return "xlalg"
	case UCI:
		return "uci"
	case EPD:
		return "epd"
	case FEN:
		return "fen"
	default:
		return fmt.Sprintf("Format(%d)", int(f))
	}
}

// ParseFormat parses a format name ("san", "source", "lalg", "halg",
// "elalg", "xlalg", "uci", "epd", "fen") into a Format.
func ParseFormat(s string) (Format, error) {
	switch s {
	case "san":
		return SAN, nil
	case "source":
		return Source, nil
	case "lalg":
		return LAlg, nil
	case "halg":
		return HAlg, nil
	case "elalg":
		return ELAlg, nil
	case "xlalg":
		return XLAlg, nil
	case "uci":
		return UCI, nil
	case "epd":
		return EPD, nil
	case "fen":
		return FEN, nil
	default:
		return 0, fmt.Errorf("output: unknown format %q", s)
	}
}

// sevenTagRoster is the fixed order and default value of the mandatory PGN
// header tags, per the STR (Seven Tag Roster).
var sevenTagRoster = []struct {
	name  string
	deflt string
}{
	{"Event", "?"},
	{"Site", "?"},
	{"Date", "????.??.??"},
	{"Round", "?"},
	{"White", "?"},
	{"Black", "?"},
	{"Result", "*"},
}

// wellKnownTagOrder is the fixed order (after the seven roster tags) in
// which the C oracle (pgn-extract's DefaultTagOrder, output.c) emits
// recognized non-roster tags when present, regardless of their appearance
// order in the source. Tags not in this list (truly unrecognized names)
// are emitted afterward in their original source appearance order.
var wellKnownTagOrder = []string{
	"WhiteTitle", "BlackTitle", "WhiteElo", "BlackElo",
	"WhiteUSCF", "BlackUSCF",
	"WhiteType", "BlackType",
	"WhiteNA", "BlackNA",
	"ECO", "NIC", "Opening", "Variation", "SubVariation",
	"LongECO",
	"TimeControl",
	"Annotator",
	"EventDate", "EventSponsor", "Section", "Stage", "Board",
	"Time", "UTCDate", "UTCTime",
	"Variant",
	"SetUp",
	"FEN",
	"Termination", "Mode", "PlyCount",
	"MaterialMatch",
}

// Options controls how Writer renders games.
type Options struct {
	Format    Format
	LineWidth int // default 75 when 0

	SevenTagRoster bool     // emit only the seven roster tags
	Roster         []string // explicit tag list; overrides SevenTagRoster

	NoComments    bool
	NoNAGs        bool
	NoVariations  bool
	NoMoveNumbers bool
	NoResults     bool
}

// Writer renders games to an underlying io.Writer according to Options.
type Writer struct {
	w    io.Writer
	opts Options
}

// NewWriter returns a Writer that writes rendered games to w according to
// opts.
func NewWriter(w io.Writer, opts Options) *Writer {
	return &Writer{w: w, opts: opts}
}

// WriteGame renders g (with replay detail r, when available) to the
// Writer's underlying io.Writer, followed by a blank line.
//
// r may be nil only when opts.Format == Source, since Source rendering
// uses only the pgn.Game data (the SAN exactly as written) and never
// consults replay output; any other format returns an error when r is
// nil, before anything is written.
func (w *Writer) WriteGame(g *pgn.Game, r *replay.Result) error {
	if r == nil && w.opts.Format != Source {
		return fmt.Errorf("output: replay result required for format %v", w.opts.Format)
	}
	switch w.opts.Format {
	case SAN, Source, LAlg, HAlg, ELAlg, XLAlg, UCI:
		return w.writeMovetextGame(g, r)
	case EPD, FEN:
		return w.writePositionGame(g, r)
	default:
		return fmt.Errorf("output: format %v not implemented", w.opts.Format)
	}
}

// writeMovetextGame renders the tag section, an optional prefix-comment
// block, and the movetext (via emitMovetext/renderMove) for every
// movetext-shaped format: SAN, Source, and the long-algebraic family
// (LAlg, HAlg, ELAlg, XLAlg, UCI).
func (w *Writer) writeMovetextGame(g *pgn.Game, r *replay.Result) error {
	if err := w.writeTags(g); err != nil {
		return err
	}
	if _, err := io.WriteString(w.w, "\n"); err != nil {
		return err
	}
	if len(g.PrefixComments) > 0 && !w.opts.NoComments && !isUCI(w.opts.Format) {
		for _, c := range g.PrefixComments {
			if _, err := io.WriteString(w.w, "{ "+c+" }\n"); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w.w, "\n"); err != nil {
			return err
		}
	}

	lw := newLineWriter(w.w, w.opts.LineWidth)
	emitMovetext(lw, g, r, w.opts)
	if err := lw.Close(); err != nil {
		return err
	}
	_, err := io.WriteString(w.w, "\n")
	return err
}

// writeTags renders the tag section: the seven roster tags (using the
// game's value if present, else the standard default) in fixed order,
// followed by any other tags in their original appearance order. When
// opts.SevenTagRoster is set, only the seven roster tags are emitted. When
// opts.Roster is set (and non-empty), it overrides SevenTagRoster and
// exactly that explicit tag list is emitted, in the order given, using the
// game's value when present (no default substitution for non-roster
// names).
func (w *Writer) writeTags(g *pgn.Game) error {
	if len(w.opts.Roster) > 0 {
		for _, name := range w.opts.Roster {
			value, _ := g.Tag(name)
			if _, err := fmt.Fprintf(w.w, "[%s %q]\n", name, value); err != nil {
				return err
			}
		}
		return nil
	}

	rosterSet := make(map[string]bool, len(sevenTagRoster))
	for _, rt := range sevenTagRoster {
		rosterSet[rt.name] = true
		value, ok := g.Tag(rt.name)
		if !ok || value == "" {
			value = rt.deflt
		}
		if _, err := fmt.Fprintf(w.w, "[%s %q]\n", rt.name, value); err != nil {
			return err
		}
	}

	if w.opts.SevenTagRoster {
		return nil
	}

	// rosterSet doubles as the "already emitted" set: well-known tags
	// print next, in the C oracle's fixed order, regardless of where they
	// appeared in the source.
	for _, name := range wellKnownTagOrder {
		value, ok := g.Tag(name)
		if !ok {
			continue
		}
		rosterSet[name] = true
		if _, err := fmt.Fprintf(w.w, "[%s %q]\n", name, value); err != nil {
			return err
		}
	}

	// Any remaining (unrecognized) tags print afterward, in original
	// source appearance order.
	for _, t := range g.Tags {
		if rosterSet[t.Name] {
			continue
		}
		rosterSet[t.Name] = true
		value, _ := g.Tag(t.Name)
		if _, err := fmt.Fprintf(w.w, "[%s %q]\n", t.Name, value); err != nil {
			return err
		}
	}
	return nil
}
