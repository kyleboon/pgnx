package pgn

import (
	"bytes"
	"fmt"
	"io"
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

// isMoveStart reports whether c may be the first character of a move
// token: a letter, a castling digit/letter (O/o/0), or '-' (for the null
// move "--").
func isMoveStart(c byte) bool {
	if c >= 'a' && c <= 'z' {
		return true
	}
	if c >= 'A' && c <= 'Z' {
		return true
	}
	return c == '-' || c == '0'
}

// isMoveChar reports whether c may appear as a non-initial character of a
// move token (the SAN alphabet plus trailing annotation glyphs).
func isMoveChar(c byte) bool {
	return moveChars[c]
}

func isAlnum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// lexBlockSize is the lexer's initial buffer, refilled with one Read when
// it runs dry. The buffer grows only when a single token (a huge comment)
// outgrows it.
const lexBlockSize = 64 << 10

// maxIntern bounds the lexer's intern table: when it holds this many
// distinct texts it is cleared and refilled, so pathological input (a new
// tag name per game, say) cannot grow it without bound and a long run
// keeps interning the texts it currently sees.
const maxIntern = 4096

// lexer turns a stream of PGN text into a sequence of tokens. It knows
// nothing about chess semantics -- it is a pure tokenizer over text.
//
// The lexer is byte-oriented, like pgn-extract's: it scans a block buffer
// of raw input bytes, so Latin-1 or any other non-UTF-8 bytes pass through
// tag values and comments unchanged. A token's text is one sub-slice of
// the buffer, converted to a string once, and the texts that repeat across
// a file (tag names, moves, move numbers, NAGs) are interned, so a game's
// tokens cost a handful of allocations rather than one per character
// group. The buffer is refilled with a single Read only when every
// buffered byte has been consumed (or a token needs more bytes), so the
// lexer never reads ahead of the token it is producing beyond what the
// reader hands it in one call, which keeps pgn.Scanner's streaming
// property (ADR 0003): a game is yielded as soon as its text is complete.
type lexer struct {
	r    io.Reader
	opts ParserOptions

	// buf[pos:end] is the unread input; buf[start:pos] is the token being
	// scanned, which a refill in the middle of a token keeps (compacting
	// the buffer to it or growing the buffer if it fills it). Between
	// tokens start == pos.
	buf   []byte
	start int
	pos   int
	end   int
	// eof is set once r has returned an error (io.EOF, or err when it is
	// another one, reported after the buffered bytes are consumed).
	eof bool
	err error

	line int
	// lineStart tracks whether the next byte to be consumed is the first
	// byte on its line, needed to implement the "% only discards at start
	// of line" rule.
	lineStart bool

	intern map[string]string
}

// newLexer creates a lexer reading from r with the given options.
func newLexer(r io.Reader, opts ParserOptions) *lexer {
	return &lexer{
		r:         r,
		opts:      opts,
		buf:       make([]byte, lexBlockSize),
		line:      1,
		lineStart: true,
		intern:    make(map[string]string),
	}
}

// fill reads more input into the buffer, keeping buf[start:end]. It
// reports whether any byte was read; false means the input is exhausted
// (l.err holds a non-EOF read error, if that is why).
func (l *lexer) fill() bool {
	if l.eof {
		return false
	}
	if l.start > 0 {
		n := copy(l.buf, l.buf[l.start:l.end])
		l.pos -= l.start
		l.end = n
		l.start = 0
	}
	if l.end == len(l.buf) {
		// The token being scanned fills the whole buffer: grow it.
		grown := make([]byte, 2*len(l.buf))
		copy(grown, l.buf[:l.end])
		l.buf = grown
	}
	for empty := 0; ; empty++ {
		n, err := l.r.Read(l.buf[l.end:])
		if n < 0 || n > len(l.buf)-l.end {
			panic("pgn: reader returned an invalid count")
		}
		l.end += n
		if err != nil {
			l.eof = true
			if err != io.EOF {
				l.err = err
			}
			return n > 0
		}
		if n > 0 {
			return true
		}
		if empty >= 100 {
			l.eof = true
			l.err = io.ErrNoProgress
			return false
		}
	}
}

// peek returns the next byte without consuming it. ok is false at EOF.
func (l *lexer) peek() (c byte, ok bool) {
	if l.pos < l.end || l.fill() {
		return l.buf[l.pos], true
	}
	return 0, false
}

// ensure buffers at least n unread bytes if the input has them, and
// reports whether it did.
func (l *lexer) ensure(n int) bool {
	for l.end-l.pos < n {
		if !l.fill() {
			return false
		}
	}
	return true
}

// text returns the token scanned so far, buf[start:pos], as a string.
func (l *lexer) text() string {
	return string(l.buf[l.start:l.pos])
}

// internText returns the token scanned so far as a string, sharing one
// string between every token with the same text. The map lookup on the
// byte slice does not allocate. Tag values and comments are not interned:
// measured over the TWIC slice, hashing them cost more than the
// allocations it saved.
func (l *lexer) internText() string {
	b := l.buf[l.start:l.pos]
	if s, ok := l.intern[string(b)]; ok {
		return s
	}
	s := string(b)
	if len(l.intern) >= maxIntern {
		clear(l.intern)
	}
	l.intern[s] = s
	return s
}

// next returns the next token from the input, or an error for malformed
// input (e.g. an unterminated string or comment).
func (l *lexer) next() (token, error) {
	for {
		l.start = l.pos
		if l.pos == l.end && !l.fill() {
			if l.err != nil {
				return token{}, l.err
			}
			return token{kind: tokEOF, line: l.line}, nil
		}
		atLineStart := l.lineStart
		ch := l.buf[l.pos]
		l.pos++
		// Every token path below ends on a byte other than a newline
		// (skipToEOL is the exception and records its own).
		l.lineStart = false

		switch {
		case ch == '\n':
			l.line++
			l.lineStart = true
			continue

		case ch == ' ' || ch == '\t' || ch == '\r':
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

		case ch == '-':
			// A '-' that does not begin the null move "--" is not a token
			// at all (lex.c, DASH case: "Single '-' not allowed"): hand it
			// to the parser as a stray token to be reported and skipped.
			if next, ok := l.peek(); ok && next == '-' {
				return l.lexMove()
			}
			return token{kind: tokStray, text: "-", line: l.line}, nil

		case isMoveStart(ch):
			return l.lexMove()

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
		if i := bytes.IndexByte(l.buf[l.pos:l.end], '\n'); i >= 0 {
			l.pos += i + 1
			l.line++
			l.lineStart = true
			return
		}
		l.pos = l.end
		l.start = l.pos // nothing to keep across the refill
		if !l.fill() {
			return
		}
	}
}

// lexTagName reads a tag name identifier following '['.
func (l *lexer) lexTagName() (token, error) {
	line := l.line
	// Skip whitespace before the tag name.
	for {
		l.start = l.pos
		ch, ok := l.peek()
		if !ok {
			return token{}, fmt.Errorf("line %d: unexpected EOF in tag", l.line)
		}
		if ch == '\n' {
			l.line++
			l.pos++
			continue
		}
		if ch == ' ' || ch == '\t' || ch == '\r' {
			l.pos++
			continue
		}
		break
	}
	for {
		ch, ok := l.peek()
		if !ok || !(isAlnum(ch) || ch == '_') {
			break
		}
		l.pos++
	}
	if l.pos == l.start {
		return token{}, fmt.Errorf("line %d: empty tag name", line)
	}
	return token{kind: tokTagName, text: l.internText(), line: line}, nil
}

// lexString reads a quoted string, having already consumed the opening
// quote. It supports \" and \\ escapes.
func (l *lexer) lexString() (token, error) {
	line := l.line
	l.start = l.pos
	for {
		for l.pos < l.end {
			switch l.buf[l.pos] {
			case '\\':
				// Keep the escape sequence exactly as written, as
				// pgn-extract's lexer does, so values round-trip byte for
				// byte on output.
				l.pos++
				next, ok := l.peek()
				if !ok {
					return token{}, fmt.Errorf("line %d: unterminated string escape", line)
				}
				if next == '\n' {
					l.line++
				}
			case '"':
				text := l.text()
				l.pos++
				return token{kind: tokString, text: text, line: line}, nil
			case '\n':
				l.line++
			}
			l.pos++
		}
		if !l.fill() {
			return token{}, fmt.Errorf("line %d: unterminated string", line)
		}
	}
}

// lexComment reads a brace-delimited comment, having already consumed the
// opening '{'. Newlines are converted to spaces. If opts.NestedComments is
// set, nested '{' increase depth and require a matching '}' to close;
// otherwise the first '}' closes the comment regardless of any '{' seen
// inside it.
func (l *lexer) lexComment() (token, error) {
	line := l.line
	l.start = l.pos
	depth := 1
	for {
		for l.pos < l.end {
			switch l.buf[l.pos] {
			case '{':
				if l.opts.NestedComments {
					depth++
				}
			case '}':
				depth--
				if depth == 0 {
					text := l.text()
					l.pos++
					return token{kind: tokComment, text: text, line: line}, nil
				}
			case '\n':
				// The buffer is the lexer's own, so the conversion is done
				// in place and the text is still one sub-slice.
				l.buf[l.pos] = ' '
				l.line++
			}
			l.pos++
		}
		if !l.fill() {
			return token{}, fmt.Errorf("line %d: unterminated comment", line)
		}
	}
}

// lexNAG reads a numeric annotation glyph, having already consumed '$'.
func (l *lexer) lexNAG() (token, error) {
	line := l.line
	l.start = l.pos
	l.consumeDigits()
	if l.pos == l.start {
		return token{}, fmt.Errorf("line %d: empty NAG", line)
	}
	return token{kind: tokNAG, text: l.internText(), line: line}, nil
}

// consumeDigits consumes a run of decimal digits.
func (l *lexer) consumeDigits() {
	for {
		ch, ok := l.peek()
		if !ok || ch < '0' || ch > '9' {
			return
		}
		l.pos++
	}
}

// lexNumberOrResult handles a leading digit (already consumed, with
// l.start pointing at it), disambiguating move numbers, zero-based
// castling notation, and game results by lookahead, matching
// gather_possible_numeric in the C lexer:
//   - '0' followed by "-1"      -> result "0-1"
//   - '0' followed by "-0-0"    -> move "0-0-0" (queenside castling)
//   - '0' followed by "-0"      -> move "0-0" (kingside castling)
//   - '1' followed by "-0"      -> result "1-0"
//   - '1' followed by "/2"      -> result "1/2-1/2" (the full form is
//     always saved, even if only "1/2" appears)
//   - otherwise                 -> move number, consuming further digits and
//     any trailing dots.
func (l *lexer) lexNumberOrResult(first byte) (token, error) {
	line := l.line

	if first == '0' {
		if l.matchAhead("-1") {
			return token{kind: tokResult, text: "0-1", line: line}, nil
		}
		if l.matchAhead("-0-0") || l.matchAhead("-0") {
			l.consumeMoveTail()
			return token{kind: tokMove, text: l.internText(), line: line}, nil
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
	l.consumeDigits()
	text := l.internText()
	// Consume any number of trailing dots (handles "1...").
	for {
		ch, ok := l.peek()
		if !ok || ch != '.' {
			break
		}
		l.pos++
	}
	return token{kind: tokMoveNumber, text: text, line: line}, nil
}

// lexMove reads a move token whose first character has been consumed
// (l.start points at it). It consumes further characters from the SAN
// move alphabet (moveChars), then any trailing '!'/'?' annotation glyphs,
// keeping the text exactly as written.
func (l *lexer) lexMove() (token, error) {
	line := l.line
	l.consumeMoveTail()
	word := l.buf[l.start:l.pos]
	// The C lexer only classifies a word as a move when it seems to be a
	// complete one (lex.c: move_seems_valid); anything else is reported
	// ("Unknown move text N.") and discarded, and the game carries on.
	// The check sees the word without its '!'/'?' glyphs, which the C
	// lexer turns into NAGs before it.
	core := len(word)
	for core > 0 && (word[core-1] == '!' || word[core-1] == '?') {
		core--
	}
	nullOrZ0 := len(word) >= 2 && ((word[0] == '-' && word[1] == '-') || (word[0] == 'Z' && word[1] == '0'))
	if !nullOrZ0 && !moveSeemsValid(word[:core]) {
		return token{kind: tokStray, text: l.text(), line: line}, nil
	}
	return token{kind: tokMove, text: l.internText(), line: line}, nil
}

// consumeMoveTail consumes the remaining characters of a move token: first
// any characters from the SAN move alphabet (moveChars), then any trailing
// '!'/'?' annotation glyphs. Shared by lexMove and the zero-form castling
// cases of lexNumberOrResult so that e.g. "0-0-0+" keeps its check suffix.
func (l *lexer) consumeMoveTail() {
	for {
		ch, ok := l.peek()
		if !ok || !isMoveChar(ch) {
			break
		}
		l.pos++
	}
	for {
		ch, ok := l.peek()
		if !ok || (ch != '!' && ch != '?') {
			return
		}
		l.pos++
	}
}

// matchAhead reports whether the upcoming input matches s exactly; if so it
// consumes those bytes, otherwise it leaves the input unchanged.
func (l *lexer) matchAhead(s string) bool {
	if !l.ensure(len(s)) || string(l.buf[l.pos:l.pos+len(s)]) != s {
		return false
	}
	l.pos += len(s)
	return true
}
