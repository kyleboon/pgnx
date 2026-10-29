package filter

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Op is the comparison a Criterion applies to a tag value.
type Op int

const (
	OpPrefix Op = iota // no operator: prefix (or, with TagMatcher.Substring, substring) match
	OpLT               // <
	OpLE               // <=
	OpNE               // <>
	OpGT               // >
	OpGE               // >=
	OpEQ               // =
	OpRegex            // =~
)

// String returns the operator's spelling in the -t file grammar ("" for
// OpPrefix).
func (o Op) String() string {
	switch o {
	case OpLT:
		return "<"
	case OpLE:
		return "<="
	case OpNE:
		return "<>"
	case OpGT:
		return ">"
	case OpGE:
		return ">="
	case OpEQ:
		return "="
	case OpRegex:
		return "=~"
	}
	return ""
}

// relational reports whether o is one of the ordered/equality operators
// (everything except OpPrefix and OpRegex).
func (o Op) relational() bool {
	return o != OpPrefix && o != OpRegex
}

// Criterion is one tag-matching requirement: Tag [Op] "Value".
//
// Tag names are case-sensitive and compared exactly against the game's
// tag names. The pseudo-tags "Player" (either White or Black), "Elo"
// (either WhiteElo or BlackElo) and "EloDiff" (absolute rating
// difference) are accepted as ordinary names and given their special
// meaning by TagMatcher.
//
// The names FEN, FENPattern and FENPatternI are positional criteria
// rather than tag criteria (see IsPositional): the C tool turns such a
// line into a sought position or pattern (taglines.c process_tag_line).
// They belong in a PositionCriteria, not a TagMatcher.
type Criterion struct {
	Tag   string
	Op    Op
	Value string
	// Label is the text following the closing quote of a FENPattern or
	// FENPatternI line, with surrounding whitespace removed; it is the
	// pattern's label (reported through the MatchLabel tag). Empty for
	// every other criterion.
	Label string

	re *regexp.Regexp // compiled Value when Op == OpRegex
}

// IsPositional reports whether c names one of the positional criteria
// FEN, FENPattern or FENPatternI, which select games by position rather
// than by tag. The CLI routes them to PositionCriteria: FEN to AddFEN,
// FENPattern to AddPattern(Value, false, Label) and FENPatternI to
// AddPattern(Value, true, Label).
func (c Criterion) IsPositional() bool {
	switch c.Tag {
	case "FEN", "FENPattern", "FENPatternI":
		return true
	}
	return false
}

// ErrPositionalCriterion is returned (wrapped) by TagMatcher.Add for a
// positional criterion (see Criterion.IsPositional).
var ErrPositionalCriterion = errors.New("positional criterion cannot be used as a tag criterion")

// ParseCriterion parses one line of pgn-extract's -t file grammar
// (taglines.c process_tag_line):
//
//	Tag [op] "value"
//
// where Tag is [A-Za-z0-9_]+, op is one of < <= <> > >= = =~ and is
// optional, whitespace around op is optional, and the value is a
// double-quoted string in which \" and \\ are escapes (other backslash
// sequences are kept verbatim, as the pgn lexer does for tag values).
// Anything after the closing quote is ignored, except that for the
// positional names FENPattern and FENPatternI the rest of the line,
// whitespace-trimmed, is the pattern's Label (taglines.c:183-211); an
// operator before a positional value is accepted and ignored, as in C.
// An =~ value must compile as a Go (RE2) regular expression; pgn-extract
// uses POSIX basic regular expressions, a documented divergence.
func ParseCriterion(line string) (Criterion, error) {
	s := strings.TrimLeft(line, " \t")
	i := 0
	for i < len(s) && isTagChar(s[i]) {
		i++
	}
	if i == 0 {
		return Criterion{}, fmt.Errorf("expected a tag name at %q", line)
	}
	c := Criterion{Tag: s[:i]}
	s = strings.TrimLeft(s[i:], " \t")

	switch {
	case strings.HasPrefix(s, "<="):
		c.Op, s = OpLE, s[2:]
	case strings.HasPrefix(s, "<>"):
		c.Op, s = OpNE, s[2:]
	case strings.HasPrefix(s, "<"):
		c.Op, s = OpLT, s[1:]
	case strings.HasPrefix(s, ">="):
		c.Op, s = OpGE, s[2:]
	case strings.HasPrefix(s, ">"):
		c.Op, s = OpGT, s[1:]
	case strings.HasPrefix(s, "=~"):
		c.Op, s = OpRegex, s[2:]
	case strings.HasPrefix(s, "="):
		c.Op, s = OpEQ, s[1:]
	}
	s = strings.TrimLeft(s, " \t")

	if !strings.HasPrefix(s, `"`) {
		return Criterion{}, fmt.Errorf("missing quoted value for tag %s in %q", c.Tag, line)
	}
	value, rest, ok := unquote(s[1:])
	if !ok {
		return Criterion{}, fmt.Errorf("missing closing quote in %q", line)
	}
	c.Value = value

	switch c.Tag {
	case "FENPattern", "FENPatternI":
		c.Label = strings.TrimSpace(rest)
		return c, nil
	case "FEN":
		return c, nil
	}
	if c.Op == OpRegex {
		re, err := regexp.Compile(c.Value)
		if err != nil {
			return Criterion{}, fmt.Errorf("invalid regular expression for tag %s: %v", c.Tag, err)
		}
		c.re = re
	}
	return c, nil
}

// unquote consumes a string body up to its closing double quote (s begins
// just after the opening quote), applying \" and \\ escapes. It returns
// the value, the remainder after the closing quote, and false when no
// closing quote was found.
func unquote(s string) (value, rest string, ok bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '\\':
			if i+1 >= len(s) {
				return "", "", false
			}
			next := s[i+1]
			if next == '"' || next == '\\' {
				b.WriteByte(next)
			} else {
				b.WriteByte(ch)
				b.WriteByte(next)
			}
			i++
		case '"':
			return b.String(), s[i+1:], true
		default:
			b.WriteByte(ch)
		}
	}
	return "", "", false
}

func isTagChar(ch byte) bool {
	return ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '_'
}

// ReadCriteria reads a -t criteria file: one ParseCriterion line per
// non-blank line. Lines whose first character is '%' are comments and
// lines that are entirely whitespace are skipped (lines.c non_blank_line);
// a trailing '\r' is tolerated so CRLF files work. A line that fails to
// parse is an error naming its 1-based line number. (pgn-extract instead
// silently stops reading at the first line that does not start with a
// tag name — a documented divergence.)
func ReadCriteria(r io.Reader) ([]Criterion, error) {
	var out []Criterion
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSuffix(sc.Text(), "\r")
		if strings.HasPrefix(line, "%") || strings.TrimSpace(line) == "" {
			continue
		}
		c, err := ParseCriterion(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		out = append(out, c)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
