package filter

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/kyleboon/pgnx/chess"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// WhoseMove restricts positional matches to positions with a particular
// side to move (--wtm / --btm; defs.h WhoseMove).
type WhoseMove int

const (
	EitherToMove WhoseMove = iota // no restriction
	WhiteToMove                   // --wtm
	BlackToMove                   // --btm
)

// DefaultPositionalDepth is the number of plies searched for a position
// when neither a -x line nor --matchplylimit raises or sets the limit
// (apply.c DEFAULT_POSITIONAL_DEPTH).
const DefaultPositionalDepth = 2000

// PositionCriteria is the set of positions a game must reach to match:
// placements from -x move lines and FEN strings, Polyglot hashes (-H)
// and FEN patterns. A game matches when any main-line or variation
// position within the search depth is in the set.
type PositionCriteria struct {
	// WhoseMove restricts matches to positions with that side to move.
	WhoseMove WhoseMove
	// MatchPlyLimit is the last ply number at which a position is tested
	// (--matchplylimit). 0 selects the C default: the depth implied by the
	// longest -x line, else DefaultPositionalDepth.
	MatchPlyLimit int
	// Patterns holds the FEN patterns (-Tf, --fenpattern(i), FENPattern
	// criteria lines).
	Patterns PatternSet

	placements map[uint64]struct{} // piece-placement keys (-x lines, FEN)
	hashes     map[uint64]struct{} // full Polyglot hashes (-H)
	lineDepth  int                 // max over -x lines of len(line)+8
}

// Empty reports whether no position, hash or pattern is sought.
func (pc *PositionCriteria) Empty() bool {
	return pc == nil || (len(pc.placements) == 0 && len(pc.hashes) == 0 && pc.Patterns.Empty())
}

// LineDepth is the search depth implied by the -x lines added so far:
// the longest line's ply count plus 8 (moves.c
// compose_positional_variation), or 0 when no line was added. The CLI
// rejects a MatchPlyLimit below it, as the C tool does (argsfile.c).
func (pc *PositionCriteria) LineDepth() int { return pc.lineDepth }

// effectiveLimit is the ply number after which the positional search
// stops (apply.c apply_move_list): MatchPlyLimit if set, else the -x
// line depth, else DefaultPositionalDepth.
func (pc *PositionCriteria) effectiveLimit() int {
	switch {
	case pc.MatchPlyLimit > 0:
		return pc.MatchPlyLimit
	case pc.lineDepth > 0:
		return pc.lineDepth
	}
	return DefaultPositionalDepth
}

func (pc *PositionCriteria) addPlacement(key uint64) {
	if pc.placements == nil {
		pc.placements = make(map[uint64]struct{})
	}
	pc.placements[key] = struct{}{}
}

// AddLine adds the position reached by playing line from the standard
// starting position (moves.c compose_positional_variation,
// store_hash_value). line is whitespace-separated SAN; move numbers are
// optional and a token starting with '*' ends it. The search depth is
// raised to the line's ply count plus 8 to catch transpositions. An
// unknown or illegal move is an error (the C tool exits 1).
func (pc *PositionCriteria) AddLine(line string) error {
	sans := positionalLineMoves(line)
	if d := len(sans) + 8; d > pc.lineDepth {
		pc.lineDepth = d
	}
	if len(sans) == 0 {
		return nil // only move numbers: nothing stored, as in C
	}
	g := &pgn.Game{Moves: make([]*pgn.Move, len(sans))}
	for i, s := range sans {
		g.Moves[i] = &pgn.Move{SAN: s}
	}
	r, err := replay.Game(g, replay.Options{})
	if err != nil {
		return fmt.Errorf("position line %q: %w", line, err)
	}
	pc.addPlacement(r.Positions[len(r.Positions)-1].Placement)
	return nil
}

// positionalLineMoves splits a -x line into its SAN tokens: leading
// digits and dots are stripped from each token (moves.c
// strip_move_number), tokens that were only a move number are dropped,
// and a token starting with '*' ends the line.
func positionalLineMoves(line string) []string {
	var out []string
	for _, tok := range strings.Fields(line) {
		if strings.HasPrefix(tok, "*") {
			break
		}
		tok = strings.TrimLeft(tok, "0123456789")
		tok = strings.TrimLeft(tok, ".")
		if tok != "" {
			out = append(out, tok)
		}
	}
	return out
}

// ReadPositionLines returns the non-blank, non-comment ('%') lines of a
// -x file, one position line each (moves.c
// add_positional_variations_from_file).
func ReadPositionLines(r io.Reader) ([]string, error) {
	var out []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if strings.HasPrefix(line, "%") || strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// AddFEN adds the piece placement of fen as a sought position (moves.c
// add_fen_positional_match). Side to move, castling rights and the
// clocks in fen are ignored for matching.
func (pc *PositionCriteria) AddFEN(fen string) error {
	b, err := chess.FromFEN(fen)
	if err != nil {
		return fmt.Errorf("position FEN %q: %w", fen, err)
	}
	pc.addPlacement(b.PlacementKey())
	return nil
}

// ErrBadHash is returned by AddHash for a value that is not 1 to 16
// hexadecimal digits.
var ErrBadHash = errors.New("hash value must be 1 to 16 hexadecimal digits")

// AddHash adds a Polyglot hash to match against (-H; apply.c
// save_polyglot_hashcode). hexval is 1 to 16 hex digits of either case.
func (pc *PositionCriteria) AddHash(hexval string) error {
	if len(hexval) == 0 || len(hexval) > 16 {
		return fmt.Errorf("%w: %q", ErrBadHash, hexval)
	}
	h, err := strconv.ParseUint(hexval, 16, 64)
	if err != nil || strings.ContainsAny(hexval, "+-_") {
		return fmt.Errorf("%w: %q", ErrBadHash, hexval)
	}
	if pc.hashes == nil {
		pc.hashes = make(map[uint64]struct{})
	}
	pc.hashes[h] = struct{}{}
	return nil
}

// AddPattern adds a FEN pattern (see PatternSet.Add); inverse also adds
// the colour-reversed form and label is reported through Evaluate's
// MatchLabel tag.
func (pc *PositionCriteria) AddPattern(pattern string, inverse bool, label string) error {
	return pc.Patterns.Add(pattern, inverse, label)
}

// ErrNotPositionalCriterion is returned (wrapped) by
// PositionCriteria.AddCriterion for a criterion that is not positional
// (see Criterion.IsPositional).
var ErrNotPositionalCriterion = errors.New("not a positional criterion")

// AddCriterion adds a positional -t line (Criterion.IsPositional): a FEN
// line through AddFEN, a FENPattern line through AddPattern without the
// inverse, a FENPatternI line with it. An unlabelled FENPatternI line
// leaves the colour-reversed pattern unlabelled rather than labelling it
// "I": taglines.c process_tag_line passes a NULL label when nothing
// follows the pattern, and add_fen_pattern then adds the reversed pattern
// with "" instead of label+"I" (fenmatcher.c:197-207). Only the
// --fenpatterni flag, whose label is the empty string rather than NULL,
// gets the "I" label.
func (pc *PositionCriteria) AddCriterion(c Criterion) error {
	switch c.Tag {
	case "FEN":
		return pc.AddFEN(c.Value)
	case "FENPattern":
		return pc.AddPattern(c.Value, false, c.Label)
	case "FENPatternI":
		if c.Label == "" {
			return pc.Patterns.add(c.Value, true, "", "")
		}
		return pc.AddPattern(c.Value, true, c.Label)
	}
	return fmt.Errorf("%s: %w", c.Tag, ErrNotPositionalCriterion)
}

// sideOK applies the WhoseMove restriction to a position.
func (pc *PositionCriteria) sideOK(c chess.Color) bool {
	switch pc.WhoseMove {
	case WhiteToMove:
		return c == chess.White
	case BlackToMove:
		return c == chess.Black
	}
	return true
}

// matches is apply.c position_matches: the placement key against the
// -x/FEN set, else the Polyglot hash against the -H set, then the
// whose-move restriction; a position found that way has the empty label.
// If that fails the patterns are tried (building the board from the
// FEN), and the whose-move restriction is applied again.
func (pc *PositionCriteria) matches(pos replay.Position) (label string, ok bool) {
	_, found := pc.placements[pos.Placement]
	if !found {
		_, found = pc.hashes[pos.Zobrist]
	}
	if found && pc.sideOK(pos.SideToMove) {
		return "", true
	}
	if pc.Patterns.Empty() {
		return "", false
	}
	b, err := chess.FromFEN(pos.FEN)
	if err != nil {
		return "", false
	}
	label, ok = pc.Patterns.Match(b)
	if !ok || !pc.sideOK(pos.SideToMove) {
		return "", false
	}
	return label, true
}

// plyNumber is the absolute ply number of the move that leads to pos:
// 1 for the standard starting position, 2*(n-1)+1 (White to move) or
// 2*(n-1)+2 (Black) for a position with fullmove number n, as play_moves
// counts plies.
func plyNumber(pos replay.Position) int {
	p := 2*(pos.FullmoveNumber-1) + 1
	if pos.SideToMove == chess.Black {
		p++
	}
	return p
}

// matchPositions is the positional part of apply.c play_moves over g's
// main line and (unless noVariations) its variations. It returns whether
// a sought position was reached, the label of the matching pattern (only
// a main-line match contributes one) and stoppedAt, the index of the
// first main-line move the walk did not process: len(r.Moves) when the
// whole game was walked, or the index at which the ply number exceeded
// the search depth with nothing matched. Evaluate uses stoppedAt to
// confine the repetition, N-move-rule and underpromotion checks and the
// end-of-game result check to the plies the C tool would have reached.
//
// Positions are tested only at ply numbers >= startPly. With a non-empty
// mark the first match is commented: a start-position match prepends the
// comment to g.PrefixComments, a match after a move appends it to that
// move (in a variation, to the variation's move). Variations are walked
// before the move they are attached to, each with its own fresh matched
// state and the default depth; a match there matches the game and
// suppresses any later main-line mark.
func matchPositions(g *pgn.Game, r *replay.Result, pc *PositionCriteria, startPly int, mark string, noVariations bool) (matched bool, label string, stoppedAt int) {
	if r == nil {
		return false, "", 0
	}
	w := &positionWalk{pc: pc, startPly: startPly, mark: mark, noVariations: noVariations}
	return w.walk(g.Moves, r, pc.effectiveLimit(), &g.PrefixComments)
}

type positionWalk struct {
	pc           *PositionCriteria
	startPly     int
	mark         string
	noVariations bool
}

// walk plays one move list (main line or variation). prefix receives a
// start-position mark; it is nil for a variation, whose start-position
// mark the C tool adds to a discarded copy of the game.
func (w *positionWalk) walk(moves []*pgn.Move, r *replay.Result, limit int, prefix *[]string) (matched bool, label string, stoppedAt int) {
	if r == nil || len(r.Positions) == 0 {
		return false, "", 0
	}
	p0 := plyNumber(r.Positions[0])
	if p0 >= w.startPly {
		if l, ok := w.pc.matches(r.Positions[0]); ok {
			matched, label = true, l
			if w.mark != "" && prefix != nil {
				*prefix = append([]string{markComment(w.mark, r.Positions[0])}, *prefix...)
			}
		}
	}

	n := len(moves)
	if len(r.Moves) < n {
		n = len(r.Moves)
	}
	i := 0
	for ; i < n && i+1 < len(r.Positions); i++ {
		ply := p0 + i
		if !matched && ply > limit {
			break
		}
		check := ply >= w.startPly

		if !w.noVariations {
			for vi, v := range moves[i].Variations {
				if vi >= len(r.Moves[i].Variations) || r.Moves[i].Variations[vi] == nil {
					continue
				}
				vm, _, _ := w.walk(v.Moves, r.Moves[i].Variations[vi], DefaultPositionalDepth, nil)
				matched = matched || vm
			}
		}

		if check && !matched {
			pos := r.Positions[i+1]
			if l, ok := w.pc.matches(pos); ok {
				matched, label = true, l
				if w.mark != "" {
					moves[i].Comments = append(moves[i].Comments, markComment(w.mark, pos))
				}
			}
		}
	}
	return matched, label, i
}
