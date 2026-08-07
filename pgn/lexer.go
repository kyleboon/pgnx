package pgn

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// moveChars classifies the characters that may appear after the first
// character of a move token, mirroring the C implementation's MoveChars
// table (lex.c, init_lex_tables). It is a superset of strict SAN to remain
// lenient, as required by the brief: files a-h, digits 0-9, piece letters,
// capture/separator/promotion characters, and castling characters.
var moveChars [256]bool

func init() {
	for c := 'a'; c <= 'z'; c++ {
		moveChars[c] = true
	}
	for c := 'A'; c <= 'Z'; c++ {
		moveChars[c] = true
	}
	for c := '0'; c <= '9'; c++ {
		moveChars[c] = true
	}
	for _, c := range "+#=:x/-" {
		moveChars[c] = true
	}
}

// isMoveStart reports whether ch may be the first character of a move
// token: a letter, a castling digit/letter (O/o/0), or '-' (for the null
// move "--").
func isMoveStart(ch rune) bool {
	if ch >= 'a' && ch <= 'z' {
		return true
	}
	if ch >= 'A' && ch <= 'Z' {
		return true
	}
	return ch == '-' || ch == '0'
}

// isMoveChar reports whether ch may appear as a non-initial character of a
// move token (the SAN alphabet plus trailing annotation glyphs).
func isMoveChar(ch rune) bool {
	return ch >= 0 && ch < 256 && moveChars[byte(ch)]
}

// lexer turns a stream of PGN text into a sequence of tokens. It knows
// nothing about chess semantics -- it is a pure tokenizer over text.
//
// Lookahead is implemented with a small explicit rune buffer (runeBuf)
// rather than bufio.Reader.UnreadRune, since UnreadRune only supports
// pushing back a single rune and this lexer sometimes needs multi-rune
// lookahead (e.g. to distinguish "1-0" results from move numbers).
type lexer struct {
	r    *bufio.Reader
	opts ParserOptions

	line int
	// runeBuf holds runes that have been read from r but pushed back for
	// re-reading, most-recently-pushed last (i.e. read from the back).
	runeBuf []rune
	// lineStart tracks whether the next rune to be consumed is the first
	// rune on its line, needed to implement the "% only discards at start
	// of line" rule.
	lineStart bool
}

// newLexer creates a lexer reading from r with the given options.
func newLexer(r io.Reader, opts ParserOptions) *lexer {
	return &lexer{
		r:         bufio.NewReader(r),
		opts:      opts,
		line:      1,
		lineStart: true,
	}
}

// readRune reads the next rune, tracking line numbers and line-start state.
func (l *lexer) readRune() (rune, error) {
	var ch rune
	if n := len(l.runeBuf); n > 0 {
		ch = l.runeBuf[n-1]
		l.runeBuf = l.runeBuf[:n-1]
	} else {
		var err error
		ch, _, err = l.r.ReadRune()
		if err != nil {
			return 0, err
		}
	}
	if ch == '\n' {
		l.line++
		l.lineStart = true
	} else {
		l.lineStart = false
	}
	return ch, nil
}

// unread pushes ch back so the next readRune/peekRune call returns it
// again. It does not attempt to restore line/lineStart bookkeeping to a
// prior state; callers only unread non-newline runes.
func (l *lexer) unread(ch rune) {
	l.runeBuf = append(l.runeBuf, ch)
}

// peekRune returns the next rune without consuming it. ok is false on EOF.
func (l *lexer) peekRune() (ch rune, ok bool) {
	if n := len(l.runeBuf); n > 0 {
		return l.runeBuf[n-1], true
	}
	r, _, err := l.r.ReadRune()
	if err != nil {
		return 0, false
	}
	l.runeBuf = append(l.runeBuf, r)
	return r, true
}

// next returns the next token from the input, or an error for malformed
// input (e.g. an unterminated string or comment).
func (l *lexer) next() (token, error) {
	for {
		atLineStart := l.lineStart
		ch, err := l.readRune()
		if err != nil {
			if err == io.EOF {
				return token{kind: tokEOF, line: l.line}, nil
			}
			return token{}, err
		}

		switch {
		case ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n':
			continue

		case ch == '[':
			return l.lexTagName()

		case ch == ']':
			// Consumed silently.
			continue

		case ch == '"':
			return l.lexString()

		case ch == '{':
			return l.lexComment()

		case ch == '}':
			// Unmatched comment end: skip silently (lenient).
			continue

		case ch == ';':
			l.skipToEOL()
			continue

		case ch == '%':
			// Only discards the line if '%' is the first character on the
			// line (lex.c: PERCENT case, checks symbol_start == line).
			if atLineStart {
				l.skipToEOL()
			}
			continue

		case ch == '$':
			return l.lexNAG()

		case ch == '(':
			return token{kind: tokRAVStart, text: "(", line: l.line}, nil

		case ch == ')':
			return token{kind: tokRAVEnd, text: ")", line: l.line}, nil

		case ch == '*':
			return token{kind: tokResult, text: "*", line: l.line}, nil

		case ch >= '0' && ch <= '9':
			return l.lexNumberOrResult(ch)

		case isMoveStart(ch):
			return l.lexMove(ch)

		default:
			// Unknown character: skip it leniently, matching pgn-extract's
			// tolerant behaviour of reporting and continuing.
			continue
		}
	}
}

// skipToEOL discards input up to and including the next newline.
func (l *lexer) skipToEOL() {
	for {
		ch, err := l.readRune()
		if err != nil || ch == '\n' {
			return
		}
	}
}

// lexTagName reads a tag name identifier following '['.
func (l *lexer) lexTagName() (token, error) {
	line := l.line
	// Skip whitespace before the tag name.
	for {
		ch, ok := l.peekRune()
		if !ok {
			return token{}, fmt.Errorf("line %d: unexpected EOF in tag", l.line)
		}
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			l.readRune()
			continue
		}
		break
	}
	var sb strings.Builder
	for {
		ch, ok := l.peekRune()
		if !ok {
			break
		}
		if isAlnum(ch) || ch == '_' {
			l.readRune()
			sb.WriteRune(ch)
			continue
		}
		break
	}
	if sb.Len() == 0 {
		return token{}, fmt.Errorf("line %d: empty tag name", line)
	}
	return token{kind: tokTagName, text: sb.String(), line: line}, nil
}

func isAlnum(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}

// lexString reads a quoted string, having already consumed the opening
// quote. It supports \" and \\ escapes.
func (l *lexer) lexString() (token, error) {
	line := l.line
	var sb strings.Builder
	for {
		ch, err := l.readRune()
		if err != nil {
			return token{}, fmt.Errorf("line %d: unterminated string", line)
		}
		if ch == '\\' {
			next, err := l.readRune()
			if err != nil {
				return token{}, fmt.Errorf("line %d: unterminated string escape", line)
			}
			switch next {
			case '"', '\\':
				sb.WriteRune(next)
			default:
				// Not a recognised escape: keep both characters as-is.
				sb.WriteRune(ch)
				sb.WriteRune(next)
			}
			continue
		}
		if ch == '"' {
			return token{kind: tokString, text: sb.String(), line: line}, nil
		}
		sb.WriteRune(ch)
	}
}

// lexComment reads a brace-delimited comment, having already consumed the
// opening '{'. Newlines are converted to spaces. If opts.NestedComments is
// set, nested '{' increase depth and require a matching '}' to close;
// otherwise the first '}' closes the comment regardless of any '{' seen
// inside it.
func (l *lexer) lexComment() (token, error) {
	line := l.line
	var sb strings.Builder
	depth := 1
	for {
		ch, err := l.readRune()
		if err != nil {
			return token{}, fmt.Errorf("line %d: unterminated comment", line)
		}
		switch ch {
		case '{':
			if l.opts.NestedComments {
				depth++
			}
			sb.WriteRune(ch)
		case '}':
			depth--
			if depth == 0 {
				return token{kind: tokComment, text: sb.String(), line: line}, nil
			}
			sb.WriteRune(ch)
		case '\n':
			sb.WriteRune(' ')
		default:
			sb.WriteRune(ch)
		}
	}
}

// lexNAG reads a numeric annotation glyph, having already consumed '$'.
func (l *lexer) lexNAG() (token, error) {
	line := l.line
	var sb strings.Builder
	for {
		ch, ok := l.peekRune()
		if !ok {
			break
		}
		if ch >= '0' && ch <= '9' {
			l.readRune()
			sb.WriteRune(ch)
			continue
		}
		break
	}
	if sb.Len() == 0 {
		return token{}, fmt.Errorf("line %d: empty NAG", line)
	}
	return token{kind: tokNAG, text: sb.String(), line: line}, nil
}

// lexNumberOrResult handles a leading digit, disambiguating move numbers,
// zero-based castling notation, and game results by lookahead, matching
// gather_possible_numeric in the C lexer:
//   - '0' followed by "-1"      -> result "0-1"
//   - '0' followed by "-0-0"    -> move "0-0-0" (queenside castling)
//   - '0' followed by "-0"      -> move "0-0" (kingside castling)
//   - '1' followed by "-0"      -> result "1-0"
//   - '1' followed by "/2"      -> result "1/2-1/2" (the full form is
//     always saved, even if only "1/2" appears)
//   - otherwise                 -> move number, consuming further digits and
//     any trailing dots.
func (l *lexer) lexNumberOrResult(first rune) (token, error) {
	line := l.line

	if first == '0' {
		if l.matchAhead("-1") {
			return token{kind: tokResult, text: "0-1", line: line}, nil
		}
		if l.matchAhead("-0-0") {
			return token{kind: tokMove, text: "0-0-0", line: line}, nil
		}
		if l.matchAhead("-0") {
			return token{kind: tokMove, text: "0-0", line: line}, nil
		}
	} else if first == '1' {
		if l.matchAhead("-0") {
			return token{kind: tokResult, text: "1-0", line: line}, nil
		}
		if l.matchAhead("/2") {
			// Optionally consume the full "-1/2" suffix too.
			l.matchAhead("-1/2")
			return token{kind: tokResult, text: "1/2-1/2", line: line}, nil
		}
	}

	// Move number: gather remaining digits.
	var sb strings.Builder
	sb.WriteRune(first)
	for {
		ch, ok := l.peekRune()
		if !ok {
			break
		}
		if ch >= '0' && ch <= '9' {
			l.readRune()
			sb.WriteRune(ch)
			continue
		}
		break
	}
	// Consume any number of trailing dots (handles "1...").
	for {
		ch, ok := l.peekRune()
		if !ok {
			break
		}
		if ch == '.' {
			l.readRune()
			continue
		}
		break
	}
	return token{kind: tokMoveNumber, text: sb.String(), line: line}, nil
}

// lexMove reads a move token, having already consumed its first character.
// It consumes further characters from the SAN move alphabet (moveChars),
// then any trailing '!'/'?' annotation glyphs, keeping the text exactly as
// written.
func (l *lexer) lexMove(first rune) (token, error) {
	line := l.line
	var sb strings.Builder
	sb.WriteRune(first)
	for {
		ch, ok := l.peekRune()
		if !ok {
			break
		}
		if isMoveChar(ch) {
			l.readRune()
			sb.WriteRune(ch)
			continue
		}
		break
	}
	for {
		ch, ok := l.peekRune()
		if !ok {
			break
		}
		if ch == '!' || ch == '?' {
			l.readRune()
			sb.WriteRune(ch)
			continue
		}
		break
	}
	return token{kind: tokMove, text: sb.String(), line: line}, nil
}

// matchAhead reports whether the upcoming input matches s exactly; if so it
// consumes those runes, otherwise it leaves the input unchanged.
func (l *lexer) matchAhead(s string) bool {
	want := []rune(s)
	got := make([]rune, 0, len(want))
	for _, w := range want {
		ch, ok := l.peekRune()
		if !ok || ch != w {
			// Push back what we consumed so far, in reverse order.
			for i := len(got) - 1; i >= 0; i-- {
				l.unread(got[i])
			}
			return false
		}
		l.readRune()
		got = append(got, ch)
	}
	return true
}
