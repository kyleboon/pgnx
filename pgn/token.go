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

	// SkipTagsOnly, when true, drops a game that has a tag section but no
	// movetext: no moves, no result token and nothing the parser
	// discarded. pgn-extract never emits such a record (grammar.c
	// parse_game), so the CLI sets this to keep game counts and numbering
	// in step with the oracle; the default keeps the record for library
	// callers. A game that ends at end of input with no moves and no
	// result token is dropped too, whatever junk came between, since the
	// C tool stops there before dealing with it. A game whose movetext
	// held only stray tokens is still yielded mid-file (with
	// Game.SkippedTokens), as the oracle emits it as a zero-move game.
	// See CONFORMANCE.md.
	SkipTagsOnly bool
}
