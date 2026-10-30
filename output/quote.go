package output

import "strings"

// quoteTagValue wraps a tag value in double quotes the way pgn-extract's
// output.c does (fprintf "[%s \"%s\"]"): bytes are written verbatim, with no
// Go-style escaping of non-printable runes such as U+00A0 or tab. pgn-extract's
// lexer keeps source escape sequences intact and the pgn package does the
// same, so existing \" and \\ pairs and lone backslashes pass through
// unchanged; only an unescaped double quote is escaped.
func quoteTagValue(v string) string {
	var sb strings.Builder
	sb.Grow(len(v) + 2)
	sb.WriteByte('"')
	for i := 0; i < len(v); i++ {
		switch c := v[i]; c {
		case '\\':
			sb.WriteByte(c)
			if i+1 < len(v) {
				i++
				sb.WriteByte(v[i])
			}
		case '"':
			sb.WriteString(`\"`)
		default:
			sb.WriteByte(c)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}
