package pgn

// tokenKind identifies the lexical category of a token produced by the
// lexer.
type tokenKind int

const (
	tokEOF tokenKind = iota
	tokTagName
	tokString
	tokComment
	tokNAG
	tokMoveNumber
	tokMove
	tokResult
	tokRAVStart
	tokRAVEnd
	// tokStray is a movetext word that is not a move (a lone '-', a bare
	// piece letter, ...). The parser reports and skips it.
	tokStray
)

func (k tokenKind) String() string {
	switch k {
	case tokEOF:
		return "EOF"
	case tokTagName:
		return "TagName"
	case tokString:
		return "String"
	case tokComment:
		return "Comment"
	case tokNAG:
		return "NAG"
	case tokMoveNumber:
		return "MoveNumber"
	case tokMove:
		return "Move"
	case tokResult:
		return "Result"
	case tokRAVStart:
		return "RAVStart"
	case tokRAVEnd:
		return "RAVEnd"
	case tokStray:
		return "Stray"
	default:
		return "Unknown"
	}
}

// token is a single lexical unit produced by the lexer.
type token struct {
	kind tokenKind
	text string
	line int
}

// ParserOptions controls lenient lexing/parsing behaviour shared between the
// pgn.lexer and the pgn.Scanner (Task 8).
type ParserOptions struct {
	// NestedComments, when true, allows "{" to nest inside a comment so that
	// only a matching "}" at depth 0 closes it. When false (the default,
	// matching pgn-extract's traditional behaviour) the first "}" closes the
	// comment regardless of any "{" seen inside it.
	NestedComments bool
}
