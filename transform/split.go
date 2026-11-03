package transform

import (
	"slices"

	"github.com/kyleboon/pgnx/pgn"
)

// SplitVariations returns the games g is emitted as under --splitvariants
// (grammar.c split_variants): first the main line, then, depth first and
// in source order, one game per variation, made of the main line up to
// the move the variation replaces followed by the variation's moves. g
// itself is left untouched; the returned games share nothing with it.
//
// No returned game has variations. A comment that followed a variation's
// closing paren moves onto the move before the variation, after that
// move's leading comments and before its NAGs (the C tool appends every
// variation's suffix_comment to the move's comment_list before
// formatting). A variation's prefix comment goes the same way onto the
// move before the branch, or onto the game's prefix comments when the
// variation replaces the first move. Every game but the first gets a
// Result tag of "*" and ends with the variation's own result token, or
// "*" when it has none.
//
// depth limits the splitting: variations nested deeper than depth levels
// are dropped rather than emitted; 0 is unlimited.
func SplitVariations(g *pgn.Game, depth int) []*pgn.Game {
	s := &splitter{src: g, depth: depth}
	s.line(g.PrefixComments, nil, g.Moves, g.Result, 0)
	return s.games
}

type splitter struct {
	src   *pgn.Game
	depth int
	games []*pgn.Game
}

// line emits the game whose moves are head (already flattened) followed
// by tail (source moves, still carrying their variations), then recurses
// into the variations of tail, as split_variants walks a level.
func (s *splitter) line(prefix []string, head, tail []*pgn.Move, result string, level int) {
	moves := slices.Clone(head)
	for _, m := range tail {
		moves = append(moves, flatten(m))
	}
	s.emit(prefix, moves, result, level)

	if s.depth != 0 && s.depth <= level {
		return
	}
	for i, m := range tail {
		for _, v := range m.Variations() {
			if len(v.Moves) == 0 {
				// split_variants skips a variation with no moves,
				// prefix comment and all.
				continue
			}
			// The variation replaces tail[i] and everything after it.
			at := len(head) + i
			branchPrefix, branch := prefix, slices.Clone(moves[:at])
			if at == 0 {
				branchPrefix = slices.Concat(prefix, v.PrefixComments)
			} else if len(v.PrefixComments) > 0 {
				last := cloneMove(branch[at-1])
				for _, c := range v.PrefixComments {
					last.AddComment(c)
				}
				branch[at-1] = last
			}
			vresult := v.Result
			if vresult == "" {
				vresult = "*"
			}
			s.line(branchPrefix, branch, v.Moves, vresult, level+1)
		}
	}
}

// emit appends the game for one line: g's tags (the Result tag "*" below
// the top level), the prefix comments and a private copy of moves.
func (s *splitter) emit(prefix []string, moves []*pgn.Move, result string, level int) {
	g := &pgn.Game{
		Tags:           slices.Clone(s.src.Tags),
		PrefixComments: slices.Clone(prefix),
		Moves:          make([]*pgn.Move, len(moves)),
		Result:         result,
		StartLine:      s.src.StartLine,
		EndLine:        s.src.EndLine,
	}
	for i, m := range moves {
		g.Moves[i] = cloneMove(m)
	}
	if level > 0 {
		g.SetTag("Result", "*")
	}
	s.games = append(s.games, g)
}

// flatten copies m without its variations, moving the comments that
// followed each variation to the end of the leading comments.
func flatten(m *pgn.Move) *pgn.Move {
	c := &pgn.Move{SAN: m.SAN, MoveNumber: m.MoveNumber}
	var gathered []pgn.Annotation
	afterVariation := false
	for _, a := range m.Annotations {
		switch a := a.(type) {
		case *pgn.Variation:
			afterVariation = true
		case pgn.Comment:
			if afterVariation {
				gathered = append(gathered, a)
			} else {
				c.Annotations = append(c.Annotations, a)
			}
		default:
			afterVariation = false
			c.Annotations = append(c.Annotations, a)
		}
	}
	for _, a := range gathered {
		c.AddComment(string(a.(pgn.Comment)))
	}
	return c
}

// cloneMove copies a flattened move so that a game can annotate it
// without the others seeing the change.
func cloneMove(m *pgn.Move) *pgn.Move {
	return &pgn.Move{SAN: m.SAN, MoveNumber: m.MoveNumber, Annotations: slices.Clone(m.Annotations)}
}
