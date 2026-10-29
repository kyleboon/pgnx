package filter

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/kyleboon/pgnx/chess"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// Colours says how a material pattern's two piece sets map onto the
// players.
type Colours int

const (
	// EitherColour tries the first set as White and as Black (-z).
	EitherColour Colours = iota
	// WhiteFirst requires the first set to be White's material (-y).
	WhiteFirst
)

// occurs is how a piece count in a pattern relates to the game
// (end.c Occurs).
type occurs int

const (
	exactly        occurs = iota // n
	numOrMore                    // n+ (or *: 0+)
	numOrLess                    // n- (or ?: 1-)
	sameAsOpponent               // =
	notSameAsOpp                 // #
	lessThanOpp                  // n<  : at least n fewer than the opponent
	moreThanOpp                  // n>  : at least n more
	lessEqOpp                    // n<= : exactly n fewer
	moreEqOpp                    // n>= : exactly n more
)

// MaterialPattern is one -z/-y material-balance specification: two
// piece sets (the second defaulting to a lone king) plus the number of
// consecutive half-moves the balance must persist for.
type MaterialPattern struct {
	num         [2][7]int    // [set][chess.PieceType]
	occ         [2][7]occurs // [set][chess.PieceType]
	numMinor    [2]int
	minorOccurs [2]occurs
	moveDepth   int
	bothColours bool
}

// ParseMaterial parses one material description (end.c decompose_line):
//
//	[depth] set1 [set2]
//
// depth is an optional integer number of half-moves the balance must be
// stable for (default 2). A set is a run of piece letters K Q R N B P
// (case-insensitive) or L (any minor piece), each followed by an
// optional single-digit count (default 1) and an optional occurrence
// suffix: * (any number), + (count or more), - (count or fewer), ? (at
// most one), = (same as opponent), # (differs from opponent), < (at least
// count fewer than opponent), <= (exactly count fewer), > (at least count
// more), >= (exactly count more). Pieces not mentioned must be absent,
// except that each side always has one king. If set2 is omitted it is a
// lone king. Text after set2 is ignored.
func ParseMaterial(line string, c Colours) (*MaterialPattern, error) {
	p := &MaterialPattern{moveDepth: 2, bothColours: c == EitherColour}
	for set := 0; set < 2; set++ {
		p.num[set][chess.King] = 1
	}

	s := strings.TrimLeft(line, " \t")
	if len(s) > 0 && isDigit(s[0]) {
		depth := 0
		for len(s) > 0 && isDigit(s[0]) {
			depth = depth*10 + int(s[0]-'0')
			s = s[1:]
		}
		s = strings.TrimLeft(s, " \t")
		p.moveDepth = depth
	}

	rest, err := p.parseSet(s, 0, line)
	if err != nil {
		return nil, err
	}
	rest = strings.TrimLeft(rest, " \t:")
	if rest != "" {
		if _, err := p.parseSet(rest, 1, line); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// parseSet is end.c extract_piece_information for one piece set.
func (p *MaterialPattern) parseSet(s string, set int, line string) (string, error) {
	for len(s) > 0 && s[0] != ' ' && s[0] != '\t' && s[0] != ':' {
		letter := upperASCII(s[0])
		s = s[1:]
		var piece chess.PieceType
		switch letter {
		case 'K':
			piece = chess.King
		case 'Q':
			piece = chess.Queen
		case 'R':
			piece = chess.Rook
		case 'N':
			piece = chess.Knight
		case 'B':
			piece = chess.Bishop
		case 'P':
			piece = chess.Pawn
		case 'L':
			piece = chess.NoPiece // minor-piece wildcard
		default:
			return "", fmt.Errorf("material pattern %q: unknown symbol %q", line, string(letter))
		}
		n, occ, rest, err := parseCombination(s, line)
		if err != nil {
			return "", err
		}
		s = rest
		if piece == chess.NoPiece {
			p.numMinor[set] = n
			p.minorOccurs[set] = occ
			continue
		}
		if piece == chess.King && n != 1 {
			n = 1 // "A king must occur exactly once."
		} else if piece == chess.Pawn && n > 8 {
			n = 8
		}
		p.num[set][piece] = n
		p.occ[set][piece] = occ
	}
	return s, nil
}

// parseCombination is end.c extract_combination: an optional single
// digit then an optional occurrence suffix.
func parseCombination(s, line string) (int, occurs, string, error) {
	n, occ := 1, exactly
	if len(s) > 0 && isDigit(s[0]) {
		n = int(s[0] - '0')
		s = s[1:]
		if len(s) > 0 && isDigit(s[0]) {
			return 0, 0, "", fmt.Errorf("material pattern %q: number > 9 is too big", line)
		}
	}
	if len(s) > 0 {
		switch s[0] {
		case '*':
			n, occ, s = 0, numOrMore, s[1:]
		case '+':
			occ, s = numOrMore, s[1:]
		case '-':
			occ, s = numOrLess, s[1:]
		case '?':
			n, occ, s = 1, numOrLess, s[1:]
		case '=':
			occ, s = sameAsOpponent, s[1:]
		case '#':
			occ, s = notSameAsOpp, s[1:]
		case '<':
			s = s[1:]
			if strings.HasPrefix(s, "=") {
				occ, s = lessEqOpp, s[1:]
			} else {
				occ = lessThanOpp
			}
		case '>':
			s = s[1:]
			if strings.HasPrefix(s, "=") {
				occ, s = moreEqOpp, s[1:]
			} else {
				occ = moreThanOpp
			}
		}
	}
	return n, occ, s, nil
}

func isDigit(ch byte) bool { return ch >= '0' && ch <= '9' }

// ReadMaterialFile reads one ParseMaterial pattern per non-blank,
// non-comment line (end.c build_endings).
func ReadMaterialFile(r io.Reader, c Colours) ([]*MaterialPattern, error) {
	var out []*MaterialPattern
	sc := bufio.NewScanner(r)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSuffix(sc.Text(), "\r")
		if strings.HasPrefix(line, "%") || strings.TrimSpace(line) == "" {
			continue
		}
		p, err := ParseMaterial(line, c)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		out = append(out, p)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// pieceCounts is the number of each piece type per side, indexed
// [side][chess.PieceType] with side 0 = White, 1 = Black.
type pieceCounts [2][7]int

func sideIndex(c chess.Color) int {
	if c == chess.Black {
		return 1
	}
	return 0
}

// countPieces tallies the pieces on b.
func countPieces(b *chess.Board) pieceCounts {
	var n pieceCounts
	for f := 0; f < 8; f++ {
		for r := 0; r < 8; r++ {
			p := b.PieceAt(chess.Square{File: f, Rank: r})
			if p.Type != chess.NoPiece {
				n[sideIndex(p.Color)][p.Type]++
			}
		}
	}
	return n
}

// pieceMatch is end.c piece_match.
func pieceMatch(available, toFind, opponents int, occ occurs) bool {
	switch occ {
	case exactly:
		return available == toFind
	case numOrMore:
		return available >= toFind
	case numOrLess:
		return available <= toFind
	case sameAsOpponent:
		return available == opponents
	case notSameAsOpp:
		return available != opponents
	case lessThanOpp:
		return available+toFind <= opponents
	case moreThanOpp:
		return available-toFind >= opponents
	case lessEqOpp:
		return available+toFind == opponents
	case moreEqOpp:
		return available-toFind == opponents
	}
	return false
}

// pieceSetMatch is end.c piece_set_match: does the game side gameSide's
// material satisfy pattern set set? An individual knight/bishop failure
// is forgiven if (and only if) the set has an L requirement that is
// then satisfied by the combined minor-piece count.
func (p *MaterialPattern) pieceSetMatch(n pieceCounts, gameSide, set int) bool {
	opp := 1 - gameSide
	match, minorFailure := true, false
	for piece := chess.Pawn; piece < chess.King && match; piece++ {
		match = pieceMatch(n[gameSide][piece], p.num[set][piece], n[opp][piece], p.occ[set][piece])
		if !match {
			if piece == chess.Knight || piece == chess.Bishop {
				minorFailure = true
				match = true
			} else {
				minorFailure = false
			}
		}
	}
	if !match {
		return false
	}
	toFind, occ := p.numMinor[set], p.minorOccurs[set]
	if toFind > 0 || occ != exactly {
		available := n[gameSide][chess.Bishop] + n[gameSide][chess.Knight]
		opponents := n[opp][chess.Bishop] + n[opp][chess.Knight]
		return pieceMatch(available, toFind, opponents, occ)
	}
	return !minorFailure
}

// materialMatch is end.c material_match: both sets must match with
// gameSide taking set 0, and the match must have held for moveDepth
// consecutive tests (tracked in depth[gameSide]) before it counts.
func (p *MaterialPattern) materialMatch(n pieceCounts, gameSide int, depth *[2]int) bool {
	match := p.pieceSetMatch(n, gameSide, 0) && p.pieceSetMatch(n, 1-gameSide, 1)
	if !match {
		depth[gameSide] = 0
		return false
	}
	if depth[gameSide] < p.moveDepth {
		depth[gameSide]++
		return false
	}
	return true
}

// matchMaterial is end.c look_for_material_match: walk the main line
// from the starting position, updating piece counts on captures and
// promotions, and test every pattern at every position (patterns are
// tried in reverse order, as the C tool prepends to its list). On the
// first match, a mark comment (if mark is non-empty) is attached to the
// move that reached the position, or to the game's prefix comments when
// it is the starting position; with addTag, a MaterialMatch tag records
// which colour took the first piece set.
func matchMaterial(g *pgn.Game, r *replay.Result, patterns []*MaterialPattern, mark string, addTag bool) bool {
	if len(patterns) == 0 {
		return true
	}
	if len(r.Positions) == 0 {
		return false
	}
	board, err := chess.FromFEN(r.Positions[0].FEN)
	if err != nil {
		return false
	}
	n := countPieces(board)
	side := sideIndex(board.SideToMove)
	depths := make([][2]int, len(patterns))

	moveIdx := -1
	for {
		whiteMatches := false
		matched := false
		for pi := len(patterns) - 1; pi >= 0; pi-- {
			p := patterns[pi]
			white := p.materialMatch(n, 0, &depths[pi])
			black := false
			if p.bothColours {
				black = p.materialMatch(n, 1, &depths[pi])
			}
			if white || black {
				matched, whiteMatches = true, white
				break
			}
		}
		if matched {
			if mark != "" {
				addMatchComment(g, moveIdx, markComment(mark, r.Positions[moveIdx+1]))
			}
			if addTag {
				colour := "Black"
				if whiteMatches {
					colour = "White"
				}
				setTag(g, "MaterialMatch", colour)
			}
			return true
		}
		if moveIdx+1 >= len(r.Moves) || moveIdx+2 >= len(r.Positions) {
			return false
		}
		mv := r.Moves[moveIdx+1]
		if mv.Captured != chess.NoPiece {
			n[1-side][mv.Captured]--
		}
		if mv.Move.Promotion != chess.NoPiece {
			n[side][mv.Move.Promotion]++
			n[side][chess.Pawn]--
		}
		moveIdx++
		side = 1 - side
	}
}

// matchPieceCount is end.c look_for_number_of_pieces: the game reaches a
// position (the start, or after a capture) with exactly target pieces,
// kings included. A match is marked like a material match.
func matchPieceCount(g *pgn.Game, r *replay.Result, target int, mark string) bool {
	if len(r.Positions) == 0 {
		return false
	}
	board, err := chess.FromFEN(r.Positions[0].FEN)
	if err != nil {
		return false
	}
	count := 0
	for _, side := range countPieces(board) {
		for _, c := range side {
			count += c
		}
	}
	moveIdx := -1
	if count != target {
		for i := range r.Moves {
			if r.Moves[i].Captured == chess.NoPiece {
				continue
			}
			count--
			if count == target {
				moveIdx = i
				break
			}
		}
		if count != target {
			return false
		}
	}
	if mark != "" && moveIdx+1 < len(r.Positions) {
		addMatchComment(g, moveIdx, markComment(mark, r.Positions[moveIdx+1]))
	}
	return true
}

// addMatchComment appends text as a comment on main-line move moveIdx,
// or to the prefix comments when moveIdx < 0 (end.c add_match_comment).
func addMatchComment(g *pgn.Game, moveIdx int, text string) {
	if moveIdx < 0 || moveIdx >= len(g.Moves) {
		g.PrefixComments = append(g.PrefixComments, text)
		return
	}
	g.Moves[moveIdx].Comments = append(g.Moves[moveIdx].Comments, text)
}

// setTag sets the last occurrence of name to value, or appends the tag.
func setTag(g *pgn.Game, name, value string) {
	for i := len(g.Tags) - 1; i >= 0; i-- {
		if g.Tags[i].Name == name {
			g.Tags[i].Value = value
			return
		}
	}
	g.Tags = append(g.Tags, pgn.Tag{Name: name, Value: value})
}
