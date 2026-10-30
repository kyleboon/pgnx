package eco

import (
	"io"

	"github.com/kyleboon/pgnx/chess"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// halfMoveLimit is pgn-extract's ECO_HALF_MOVE_LIMIT (eco.c): a game
// position may be classified by a line up to this many plies shorter, and
// classification stops this many plies past the longest line in the table.
const halfMoveLimit = 6

// tagNames lists the tags an ECO line may carry, in the order Apply writes
// them (apply.c:1301-1314).
var tagNames = [4]string{"ECO", "Opening", "Variation", "SubVariation"}

// Entry is the classification carried by one ECO line. A tag is written
// to a game only if the line had it; an empty string with the tag present
// is written as an empty tag, as the C tool does.
type Entry struct {
	ECO, Opening, Variation, SubVariation string
	present                               [4]bool
}

// Tags returns the tags present in e, in ECO, Opening, Variation,
// SubVariation order.
func (e Entry) Tags() []pgn.Tag {
	values := [4]string{e.ECO, e.Opening, e.Variation, e.SubVariation}
	var out []pgn.Tag
	for i, name := range tagNames {
		if e.present[i] {
			out = append(out, pgn.Tag{Name: name, Value: values[i]})
		}
	}
	return out
}

// entry is a stored ECO line: the keys that identify its final position
// plus the tags to apply.
type entry struct {
	halfMoves  int         // ply number of the final position
	toMove     chess.Color // side to move in the final position
	cumulative uint64      // wrapping sum of the placement key after each move
	Entry
}

// Table is a loaded ECO classification table. It is read-only after Load
// and safe for concurrent Classify calls.
type Table struct {
	byKey        map[uint64][]entry // placement key → entries in file order
	maxHalfMoves int                // longest line + halfMoveLimit
	n            int

	// Skipped counts lines whose moves could not be replayed (an illegal
	// or unresolvable move, or an invalid FEN tag); they are left out of
	// the table, as the C tool leaves them out after logging the failure.
	Skipped int
}

// Load builds a Table from a PGN file of ECO lines: each game's ECO,
// Opening, Variation and SubVariation tags are recorded against the
// position its main line reaches (a FEN tag is honoured as the starting
// position). Variations in a line are ignored. A line reaching a position
// already recorded with the same ply count, side to move and move order
// is skipped, keeping the first (the C tool logs an "ECO hash collision").
// Lines that fail replay are skipped and counted in Table.Skipped. A fatal
// scanner error is returned.
func Load(r io.Reader) (*Table, error) {
	t := &Table{byKey: make(map[uint64][]entry), maxHalfMoves: halfMoveLimit}
	sc := pgn.NewScanner(r, pgn.ParserOptions{})
	for sc.Next() {
		g := sc.Game()
		res, err := replay.Game(g, replay.Options{})
		if err != nil {
			t.Skipped++
			continue
		}
		t.add(g, res)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return t, nil
}

// Len returns the number of lines stored in the table.
func (t *Table) Len() int {
	if t == nil {
		return 0
	}
	return t.n
}

// add records one replayed ECO line (eco.c save_eco_details).
func (t *Table) add(g *pgn.Game, res *replay.Result) {
	final := res.Positions[len(res.Positions)-1]
	key := final.Placement
	half := res.PlyCount()
	var cum uint64
	for _, p := range res.Positions[1:] {
		cum += p.Placement
	}
	for _, e := range t.byKey[key] {
		if e.halfMoves == half && e.toMove == final.SideToMove && e.cumulative == cum {
			return // duplicate move sequence: first occurrence wins
		}
	}
	if half+halfMoveLimit > t.maxHalfMoves {
		t.maxHalfMoves = half + halfMoveLimit
	}
	e := entry{halfMoves: half, toMove: final.SideToMove, cumulative: cum}
	values := [4]*string{&e.ECO, &e.Opening, &e.Variation, &e.SubVariation}
	for i, name := range tagNames {
		if v, ok := g.Tag(name); ok {
			*values[i] = v
			e.present[i] = true
		}
	}
	t.byKey[key] = append(t.byKey[key], e)
	t.n++
}

// Classify finds the ECO line matching a replayed game (apply.c:1196 and
// eco.c eco_matches). Each main-line position after a move is looked up;
// the match found at the latest ply is returned. The start position is
// never tested and variations are ignored. It returns false when no
// position matched, when the table is nil or when r has no moves.
//
// At a given ply, with half the ply number of the position and cum the
// wrapping sum of the placement keys reached so far: positions beyond the
// longest line plus six plies are not tested; among lines reaching the
// same placement, one with the same ply number, side to move and cum is
// an exact match and wins; otherwise the earliest-loaded line with the
// same side to move and a ply number at most six less than half is taken.
func (t *Table) Classify(r *replay.Result) (Entry, bool) {
	if t == nil || r == nil {
		return Entry{}, false
	}
	var (
		cum   uint64
		found *entry
	)
	for _, p := range r.Positions[1:] {
		cum += p.Placement
		half := plyNumber(p)
		if half > t.maxHalfMoves {
			break // the ply number only grows
		}
		if e := t.match(p.Placement, half, p.SideToMove, cum); e != nil {
			found = e
		}
	}
	if found == nil {
		return Entry{}, false
	}
	return found.Entry, true
}

// match is eco_matches for one position. The C chain is LIFO and its loop
// keeps overwriting the partial candidate, so the earliest-loaded partial
// match is the one it returns; iterating in file order and keeping the
// first candidate reproduces that.
func (t *Table) match(key uint64, half int, toMove chess.Color, cum uint64) *entry {
	var possible *entry
	cands := t.byKey[key]
	for i := range cands {
		e := &cands[i]
		if e.halfMoves == half && e.toMove == toMove && e.cumulative == cum {
			return e
		}
		if diff := half - e.halfMoves; diff >= 0 && diff <= halfMoveLimit && e.toMove == toMove && possible == nil {
			possible = e
		}
	}
	return possible
}

// plyNumber is apply.c half_moves_played: the absolute ply number of a
// position, from its fullmove number and side to move.
func plyNumber(p replay.Position) int {
	half := 2 * (p.FullmoveNumber - 1)
	if p.SideToMove == chess.Black {
		half++
	}
	return half
}

// Apply replaces g's ECO classification with e: every ECO, Opening,
// Variation and SubVariation tag is removed, then those present in e are
// appended in that order (apply.c:1282-1314). Other tags keep their order.
func Apply(g *pgn.Game, e Entry) {
	if g == nil {
		return
	}
	kept := make([]pgn.Tag, 0, len(g.Tags)+4)
	for _, tag := range g.Tags {
		if !isECOTag(tag.Name) {
			kept = append(kept, tag)
		}
	}
	g.Tags = append(kept, e.Tags()...)
}

func isECOTag(name string) bool {
	for _, n := range tagNames {
		if n == name {
			return true
		}
	}
	return false
}

// FilePrefix returns the base name of the file an -E/--eco-split game is
// written to (eco.c open_eco_output_file): "noeco" when ecoTag is empty or
// does not start with an ASCII letter, otherwise the first level bytes of
// ecoTag (all of it when shorter). A level below 1 also yields "noeco",
// as the C tool's internal-error branch does.
func FilePrefix(ecoTag string, level int) string {
	if ecoTag == "" || !isASCIILetter(ecoTag[0]) || level < 1 {
		return "noeco"
	}
	if level > len(ecoTag) {
		return ecoTag
	}
	return ecoTag[:level]
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
