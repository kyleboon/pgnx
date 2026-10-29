package filter

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/kyleboon/pgnx/pgn"
)

// A seqMove is one half-move of a Sequence as written in the -v line: an
// ordinary SAN move (possibly "a|b" alternatives), an any-move ("*"), or
// a disallowed move ("!Nf3"). The C tool classifies by the first
// character only, so "*|c4" is an any-move that can also match c4 by
// text; that quirk is kept (moves.c compose_variation, permutation_match).
type seqMove struct {
	text       string
	any        bool
	disallowed bool
}

// Sequence is a move sequence from a -v line (pgn-extract's
// variation_list): the game's moves must match it from its first move,
// in any order (permutation) or exactly (straight). Half-moves at even
// indices are White's, odd Black's.
type Sequence struct {
	moves              []seqMove
	whiteAny, blackAny int
	whiteDis, blackDis int
}

// Len returns the number of half-moves in the sequence.
func (s *Sequence) Len() int { return len(s.moves) }

// ErrNoMoves is returned by ParseSequence for a line that has tokens but
// no moves (for example only move numbers). The C tool keeps such a line
// as an empty sequence that every game matches; pgnx rejects it.
var ErrNoMoves = errors.New("sequence has no moves")

// ParseSequence parses one -v line (moves.c compose_variation). Tokens
// are whitespace separated; a leading move number ("1." or "1...") is
// stripped and a number-only token skipped; a token starting with '*'
// matches any move and one starting with '!' is a move that must not be
// played at that point; "a|b" offers alternatives. A blank or '%'
// comment line yields a nil Sequence.
func ParseSequence(line string) (*Sequence, error) {
	line = strings.TrimSuffix(line, "\r")
	if strings.HasPrefix(line, "%") || strings.TrimSpace(line) == "" {
		return nil, nil
	}
	s := &Sequence{}
	for _, tok := range strings.Fields(line) {
		tok = stripMoveNumber(tok)
		if tok == "" {
			continue
		}
		m := seqMove{text: tok, any: tok[0] == '*', disallowed: tok[0] == '!'}
		black := len(s.moves)%2 == 1
		switch {
		case m.any && black:
			s.blackAny++
		case m.any:
			s.whiteAny++
		case m.disallowed && black:
			s.blackDis++
		case m.disallowed:
			s.whiteDis++
		}
		s.moves = append(s.moves, m)
	}
	if len(s.moves) == 0 {
		return nil, fmt.Errorf("%w: %q", ErrNoMoves, line)
	}
	return s, nil
}

// stripMoveNumber removes leading digits and dots (moves.c
// strip_move_number); the result is empty for a number-only token.
func stripMoveNumber(tok string) string {
	i := 0
	for i < len(tok) && tok[i] >= '0' && tok[i] <= '9' {
		i++
	}
	for i < len(tok) && tok[i] == '.' {
		i++
	}
	return tok[i:]
}

// ReadSequences parses a -v file: one Sequence per non-blank,
// non-comment line (moves.c add_textual_variations_from_file). The C
// tool prepends each line to its list, but only "some sequence matches"
// is observable, so file order is kept.
func ReadSequences(r io.Reader) ([]*Sequence, error) {
	var out []*Sequence
	sc := bufio.NewScanner(r)
	n := 0
	for sc.Scan() {
		n++
		s, err := ParseSequence(sc.Text())
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		if s != nil {
			out = append(out, s)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// isMoveChar is the C tool's rough notion of a character that can be
// part of a move: ASCII letter, digit or '-' (moves.c move_char).
func isMoveChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-'
}

// textualMatch reports whether actual occurs in variationMove bounded on
// both sides by non-move characters (or the string ends), so that
// "Nc6|Nf3|f3" matches "f3" but not "c6" (moves.c
// textual_variation_match).
func textualMatch(variationMove, actual string) bool {
	if actual == "" {
		return false
	}
	for off := 0; off <= len(variationMove); {
		i := strings.Index(variationMove[off:], actual)
		if i < 0 {
			return false
		}
		pos := off + i
		end := pos + len(actual)
		if (pos == 0 || !isMoveChar(variationMove[pos-1])) &&
			(end == len(variationMove) || !isMoveChar(variationMove[end])) {
			return true
		}
		// Skip past the run of move characters we landed in.
		p := pos
		for p < len(variationMove) && isMoveChar(variationMove[p]) {
			p++
		}
		if p == pos {
			p++
		}
		off = p
	}
	return false
}

// straight reports whether game (its moves from the chosen starting
// point) begins with s, move for move (moves.c straight_match). An
// any-move accepts whatever was played; a disallowed move fails the
// match only if the move played matches its text.
func (s *Sequence) straight(game []string) bool {
	i := 0
	for i < len(s.moves) && i < len(game) {
		m := s.moves[i]
		if !m.any && textualMatch(m.text, game[i]) == m.disallowed {
			return false
		}
		i++
	}
	// The game may be shorter than the sequence.
	return i == len(s.moves)
}

// permutation reports whether game begins with the moves of s in any
// order, White's moves matching White's and Black's matching Black's
// (moves.c permutation_match). Stage one scans the first Len() game
// moves for any disallowed move of the right colour and fails on one;
// surviving disallowed entries then count as any-moves. Stage two
// consumes game moves one at a time, each against an unmatched entry of
// its colour, falling back on that colour's any-move slack.
func (s *Sequence) permutation(game []string) bool {
	n := len(s.moves)
	matched := make([]bool, n)
	whiteAny, blackAny := s.whiteAny, s.blackAny

	if s.whiteDis > 0 || s.blackDis > 0 {
		white := true
		for k := 0; k < n && k < len(game); k++ {
			start := 0
			if !white {
				start = 1
			}
			for vi := start; vi < n; vi += 2 {
				if s.moves[vi].disallowed && textualMatch(s.moves[vi].text, game[k]) {
					return false
				}
			}
			white = !white
		}
		for vi, m := range s.moves {
			if m.disallowed {
				matched[vi] = true
				if vi%2 == 0 {
					whiteAny++
				} else {
					blackAny++
				}
			}
		}
	}

	consumed := 0
	white := true
	for k := 0; consumed < n && k < len(game); k++ {
		ok := false
		start := 0
		if !white {
			start = 1
		}
		for vi := start; vi < n; vi += 2 {
			if !matched[vi] && textualMatch(s.moves[vi].text, game[k]) {
				matched[vi] = true
				ok = true
				break
			}
		}
		if !ok {
			switch {
			case white && whiteAny > 0:
				whiteAny--
				ok = true
			case !white && blackAny > 0:
				blackAny--
				ok = true
			}
		}
		if !ok {
			return false
		}
		consumed++
		white = !white
	}
	return consumed == n
}

// matches applies the configured matcher to game.
func (s *Sequence) matches(game []string, exact bool) bool {
	if exact {
		return s.straight(game)
	}
	return s.permutation(game)
}

// sequenceMoves returns the game's main-line moves as the text the
// sequences are matched against. As in the C tool, this is the move as
// written in the source, not canonical SAN (pgn-extract rewrites moves
// only at output time, after check_textual_variations has run), so an
// over-disambiguated "Ngf3" matches "Ngf3" and not "Nf3". Trailing '!'
// and '?' glyphs are not part of the move (the C lexer returns them as
// NAG tokens), and any written check symbols ("+", "#", "++", "+#")
// become a single "+" (grammar.c parse_move_unit). A check that was not
// written gets no "+", so "Bb5" in the game matches both "Bb5" and
// "Bb5+" in a sequence, while "Bb5+" in the game matches only "Bb5+".
func sequenceMoves(g *pgn.Game) []string {
	out := make([]string, len(g.Moves))
	for i, m := range g.Moves {
		out[i] = sequenceText(m.SAN)
	}
	return out
}

// sequenceText normalises one written move for sequence matching; see
// sequenceMoves.
func sequenceText(san string) string {
	san = strings.TrimRight(san, "!?")
	core := strings.TrimRight(san, "+#")
	if len(core) < len(san) {
		return core + "+"
	}
	return core
}

// matchSequences reports whether any of seqs matches the game's main
// line (moves.c check_textual_variations): straight when exact,
// otherwise as a permutation; from the first move, or with anywhere from
// any starting move while at least one sequence still fits in the moves
// that remain. Colours are taken from the index within the window, as in
// C, so a window starting on a Black move treats it as White's.
func matchSequences(g *pgn.Game, seqs []*Sequence, exact, anywhere bool) bool {
	if len(seqs) == 0 {
		return true
	}
	game := sequenceMoves(g)
	if !anywhere {
		for _, s := range seqs {
			if s.matches(game, exact) {
				return true
			}
		}
		return false
	}
	for start := 0; start < len(game); start++ {
		window := game[start:]
		fits := false
		for _, s := range seqs {
			if s.Len() <= len(window) {
				fits = true
				if s.matches(window, exact) {
					return true
				}
			}
		}
		if !fits {
			return false
		}
	}
	return false
}
