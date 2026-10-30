package filter

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kyleboon/pgnx/chess"
)

// ErrDuplicatePattern is returned by PatternSet.Add when the pattern is
// already in the set. The set is unchanged; the first-added label wins.
// Callers may log it (pgn-extract warns) and otherwise carry on.
var ErrDuplicatePattern = errors.New("duplicate FEN pattern")

// Pattern metacharacters (fenmatcher.c).
const (
	patEmpty    = '_' // an empty square in the rendered rank
	patNonEmpty = '!' // any occupied square
	patAny      = '?' // any square state
	patStar     = '*' // zero or more squares of any state
	patWhite    = 'A' // any white piece
	patBlack    = 'a' // any black piece
	patNotPawn  = 'm' // anything but a pawn (empty included)
	patCCLStart = '['
	patCCLEnd   = ']'
	patNCCL     = '^'
	patMaterial = ':' // introduces a material constraint
)

// patternNode is one rank of a FEN pattern in the pattern trie
// (fenmatcher.c FENPatternMatch). Patterns sharing leading ranks share
// nodes; alt chains patterns that diverge at this rank. The label and
// constraint live on the final (rank 1) node.
type patternNode struct {
	rank       string
	label      string
	constraint *MaterialPattern
	alt, next  *patternNode
}

// PatternSet is a collection of FEN patterns (--fenpattern,
// --fenpatterni and FENPattern/FENPatternI criteria lines), stored as a
// rank trie in insertion order. The zero value is an empty set.
//
// A pattern is eight rank patterns, rank 8 first, separated by '/'. Each
// rank pattern is matched against the rank rendered as eight characters,
// file a to h, with piece letters as in FEN and '_' for an empty square:
//
//	?        any square
//	!        any occupied square
//	A / a    any white / black piece
//	m        anything except a pawn
//	*        zero or more squares of any state
//	1-8      exactly that many empty squares
//	[...]    any of the listed characters (metacharacters allowed)
//	[^...]   none of the listed characters
//	other    that exact character (piece letter or '_')
//
// The whole rank must be consumed. Text after a space is ignored, so a
// full FEN may be given. Text after ':' is a material constraint in the
// -z/-y grammar that the position must also satisfy.
type PatternSet struct {
	root *patternNode
}

// Empty reports whether the set holds no patterns.
func (s *PatternSet) Empty() bool { return s == nil || s.root == nil }

// Add parses pattern and inserts it (fenmatcher.c add_fen_pattern). When
// inverse is true the colour-reversed pattern (ranks mirrored, piece
// colours swapped) is added as well, labelled label+"I" (so "I" for an
// unlabelled pattern, as the C tool's --fenpatterni does); its material
// constraint, if any, is the same constraint, which is then matched for
// either colour.
//
// A malformed pattern or constraint returns an error and leaves the set
// unchanged. A pattern already present returns ErrDuplicatePattern.
func (s *PatternSet) Add(pattern string, inverse bool, label string) error {
	return s.add(pattern, inverse, label, label+"I")
}

// add is Add with an explicit label for the colour-reversed pattern.
func (s *PatternSet) add(pattern string, inverse bool, label, inverseLabel string) error {
	ranks, rest, err := splitPattern(pattern)
	if err != nil {
		return err
	}
	var constraint *MaterialPattern
	if strings.HasPrefix(rest, string(patMaterial)) {
		spec := rest[1:]
		if strings.TrimSpace(spec) != "" {
			colours := WhiteFirst
			if inverse {
				colours = EitherColour
			}
			constraint, err = ParseMaterial(spec, colours)
			if err != nil {
				return fmt.Errorf("FEN pattern %q: %w", pattern, err)
			}
		}
	}

	err = s.insert(ranks, label, constraint)
	if !inverse {
		return err
	}
	rranks, _, rerr := splitPattern(reverseFENPattern(strings.Join(ranks, "/")))
	if rerr != nil {
		return rerr // cannot happen: reversal preserves well-formedness
	}
	if ierr := s.insert(rranks, inverseLabel, constraint); ierr != nil && err == nil {
		err = ierr
	}
	return err
}

// splitPattern validates the board part of pattern and returns its eight
// rank patterns and the unconsumed remainder (starting at ' ' or ':', or
// empty). It mirrors the syntax scan in add_fen_pattern.
func splitPattern(pattern string) ([]string, string, error) {
	bad := func(reason string) ([]string, string, error) {
		return nil, "", fmt.Errorf("FEN pattern %q badly formed: %s", pattern, reason)
	}
	var ranks []string
	rankStart := 0
	rankSymbols := 0
	inClosure := false
	i := 0
	for ; i < len(pattern); i++ {
		ch := pattern[i]
		if ch == ' ' || ch == patMaterial {
			break
		}
		switch ch {
		case '/':
			if rankSymbols == 0 {
				return bad("empty rank")
			}
			ranks = append(ranks, pattern[rankStart:i])
			rankStart = i + 1
			rankSymbols = 0
		case patCCLStart:
			if inClosure {
				return bad("nested closures not allowed")
			}
			inClosure = true
		case patCCLEnd:
			if !inClosure {
				return bad("missing [ to match ]")
			}
			inClosure = false
		case patNCCL:
			if !inClosure {
				return bad("^ not allowed outside [...]")
			}
		default:
			rankSymbols++
		}
	}
	if len(ranks) != 7 {
		return bad("need exactly 8 ranks")
	}
	if rankSymbols == 0 {
		return bad("empty rank")
	}
	if inClosure {
		return bad("missing ] to match [")
	}
	ranks = append(ranks, pattern[rankStart:i])
	return ranks, pattern[i:], nil
}

// insert adds a rank list to the trie (pattern_tree_insert and
// insert_pattern): walk while ranks are equal; a pattern equal in every
// rank is a duplicate; otherwise the remainder hangs off the alt chain
// at the first differing rank.
func (s *PatternSet) insert(ranks []string, label string, constraint *MaterialPattern) error {
	var head *patternNode
	tail := &head
	for i, r := range ranks {
		n := &patternNode{rank: r}
		if i == len(ranks)-1 {
			n.label = label
			n.constraint = constraint
		}
		*tail = n
		tail = &n.next
	}
	if s.root == nil {
		s.root = head
		return nil
	}
	return insertPattern(s.root, head)
}

func insertPattern(node, next *patternNode) error {
	for node.rank == next.rank {
		if node.next == nil {
			return ErrDuplicatePattern
		}
		node, next = node.next, next.next
	}
	if node.alt != nil {
		return insertPattern(node.alt, next)
	}
	node.alt = next
	return nil
}

// reverseFENPattern inverts the colour sense of a pattern's board part
// (fenmatcher.c reverse_fen_pattern): ranks in reverse order with the
// case of every piece letter swapped. The colour-neutral 'm' is kept as
// is (C upper-cases it, which can never match; see CONFORMANCE.md).
func reverseFENPattern(pattern string) string {
	ranks := strings.Split(pattern, "/")
	for i, j := 0, len(ranks)-1; i < j; i, j = i+1, j-1 {
		ranks[i], ranks[j] = ranks[j], ranks[i]
	}
	var sb strings.Builder
	sb.Grow(len(pattern))
	for i, r := range ranks {
		if i > 0 {
			sb.WriteByte('/')
		}
		for k := 0; k < len(r); k++ {
			ch := r[k]
			switch {
			case ch == patNotPawn:
				sb.WriteByte(ch)
			case ch >= 'a' && ch <= 'z':
				sb.WriteByte(ch - 'a' + 'A')
			case ch >= 'A' && ch <= 'Z':
				sb.WriteByte(ch - 'A' + 'a')
			default:
				sb.WriteByte(ch)
			}
		}
	}
	return sb.String()
}

// Match tests b against the set (pattern_match_board). It returns the
// label of the first matching pattern in trie walk order, which is the
// empty string for an unlabelled pattern, and ok=false when nothing
// matches. Ranks are rendered lazily, rank 8 first.
func (s *PatternSet) Match(b *chess.Board) (label string, ok bool) {
	if s.Empty() {
		return "", false
	}
	var ranks [8]string
	return matchRank(b, s.root, 0, &ranks)
}

// matchRank is pattern_match_rank: ranks[idx] holds rank 8-idx rendered
// on demand. At the last rank a constraint must also hold; on any
// failure the alternatives at this depth are tried in order.
func matchRank(b *chess.Board, node *patternNode, idx int, ranks *[8]string) (string, bool) {
	if ranks[idx] == "" {
		ranks[idx] = rankText(b, 7-idx)
	}
	for ; node != nil; node = node.alt {
		if !matchHere(node.rank, ranks[idx]) {
			continue
		}
		if idx == 7 {
			if node.constraint == nil || node.constraint.MatchBoard(b) {
				return node.label, true
			}
			continue
		}
		if label, ok := matchRank(b, node.next, idx+1, ranks); ok {
			return label, true
		}
	}
	return "", false
}

// rankText renders rank (0-based) as eight characters, file a to h, with
// FEN piece letters and '_' for an empty square (convert_rank_to_text).
func rankText(b *chess.Board, rank int) string {
	var buf [8]byte
	for f := 0; f < 8; f++ {
		p := b.PieceAt(chess.Square{File: f, Rank: rank})
		buf[f] = pieceLetter(p)
	}
	return string(buf[:])
}

func pieceLetter(p chess.Piece) byte {
	var ch byte
	switch p.Type {
	case chess.Pawn:
		ch = 'p'
	case chess.Knight:
		ch = 'n'
	case chess.Bishop:
		ch = 'b'
	case chess.Rook:
		ch = 'r'
	case chess.Queen:
		ch = 'q'
	case chess.King:
		ch = 'k'
	default:
		return patEmpty
	}
	if p.Color == chess.White {
		ch = ch - 'a' + 'A'
	}
	return ch
}

// matchHere anchors re at the start of text and requires it to consume
// all of text (fenmatcher.c matchhere).
func matchHere(re, text string) bool {
	if re == "" && text == "" {
		return true
	}
	if re != "" && re[0] == patStar {
		return matchStar(re[1:], text)
	}
	if re == "" || text == "" {
		return false
	}
	switch c := re[0]; c {
	case patAny:
		return matchHere(re[1:], text[1:])
	case patNonEmpty, patWhite, patBlack, patNotPawn:
		return matchOne(c, text[0]) && matchHere(re[1:], text[1:])
	case patCCLStart:
		if len(re) > 1 && re[1] == patNCCL {
			return matchNCCL(re[2:], text)
		}
		return matchCCL(re[1:], text)
	case '1', '2', '3', '4', '5', '6', '7', '8':
		empty := int(c - '0')
		if len(text) < empty {
			return false
		}
		for k := 0; k < empty; k++ {
			if text[k] != patEmpty {
				return false
			}
		}
		return matchHere(re[1:], text[empty:])
	default:
		return c == text[0] && matchHere(re[1:], text[1:])
	}
}

// matchStar is leftmost-longest: try the remaining pattern from the end
// of the rank backwards to the current square (fenmatcher.c matchstar).
func matchStar(re, text string) bool {
	for i := len(text); i >= 0; i-- {
		if matchHere(re, text[i:]) {
			return true
		}
	}
	return false
}

// matchOne reports whether the single pattern character rc accepts the
// rendered square tc (fenmatcher.c matchone).
func matchOne(rc, tc byte) bool {
	if rc == tc {
		return true
	}
	switch rc {
	case patNonEmpty:
		return tc != patEmpty
	case patWhite:
		return tc >= 'A' && tc <= 'Z'
	case patBlack:
		return tc >= 'a' && tc <= 'z'
	case patAny:
		return true
	case patNotPawn:
		return tc != 'P' && tc != 'p'
	}
	return false
}

// matchCCL matches [...]: re starts just after '['. Any listed character
// (metacharacters included) may accept the square.
func matchCCL(re, text string) bool {
	i := 0
	for i < len(re) && re[i] != patCCLEnd && !matchOne(re[i], text[0]) {
		i++
	}
	if i >= len(re) || re[i] == patCCLEnd {
		return false
	}
	for i < len(re) && re[i] != patCCLEnd {
		i++
	}
	if i >= len(re) {
		return false
	}
	return matchHere(re[i+1:], text[1:])
}

// matchNCCL matches [^...]: re starts just after "[^". No listed
// character may accept the square.
func matchNCCL(re, text string) bool {
	i := 0
	for i < len(re) && re[i] != patCCLEnd && !matchOne(re[i], text[0]) {
		i++
	}
	if i >= len(re) || re[i] != patCCLEnd {
		return false
	}
	return matchHere(re[i+1:], text[1:])
}

// MatchBoard reports whether b's material satisfies p at this single
// position, ignoring the pattern's depth (end.c
// constraint_material_match): the first piece set is tried as White,
// and also as Black when the pattern was parsed with EitherColour.
func (p *MaterialPattern) MatchBoard(b *chess.Board) bool {
	n := countPieces(b)
	if p.pieceSetMatch(n, 0, 0) && p.pieceSetMatch(n, 1, 1) {
		return true
	}
	return p.bothColours && p.pieceSetMatch(n, 1, 0) && p.pieceSetMatch(n, 0, 1)
}
