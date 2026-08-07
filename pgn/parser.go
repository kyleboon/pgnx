package pgn

import "strconv"

// parser turns a token stream from the lexer into Games. It implements a
// small recursive-descent grammar:
//
//	game      = { tag } movetext
//	movetext  = { comment | nag | move | variation } [ result ]
//	variation = "(" movetext ")"
//
// A game never fails to parse outright: unexpected tokens are recorded as
// recoverable errors on the Game and skipped. Only I/O errors and lexer
// errors (malformed low-level tokens such as an unterminated string) are
// fatal and propagate to the caller.
type parser struct {
	lex *lexer

	// pending holds a single pushed-back token, used when a tokTagName
	// terminates the previous game and must start the next one.
	pending    *token
	pendingErr error
}

func newParser(lex *lexer) *parser {
	return &parser{lex: lex}
}

// pushBack stashes tok so the next call to nextToken returns it again.
func (p *parser) pushBack(tok token) {
	t := tok
	p.pending = &t
}

// nextToken returns the next token, consuming the pushed-back token first
// if there is one.
func (p *parser) nextToken() (token, error) {
	if p.pending != nil {
		tok := *p.pending
		p.pending = nil
		return tok, nil
	}
	return p.lex.next()
}

// parseGame parses a single game starting at the current position. It
// returns (nil, nil) at a clean EOF with nothing left to parse, and
// (nil, err) on a fatal lexer/IO error. Otherwise it returns a non-nil
// Game, possibly with recoverable errors recorded via Game.errs.
func (p *parser) parseGame() (*Game, error) {
	g := &Game{}

	tok, err := p.nextToken()
	if err != nil {
		return nil, err
	}
	if tok.kind == tokEOF {
		return nil, nil
	}
	g.StartLine = tok.line

	// Tag section.
	for tok.kind == tokTagName {
		tagName := tok.text
		valTok, err := p.nextToken()
		if err != nil {
			return nil, err
		}
		if valTok.kind != tokString {
			g.errs = append(g.errs, &ParseError{Line: valTok.line, Msg: "expected tag value string after tag name " + tagName})
			// Leave valTok to be reprocessed as whatever it is; don't lose
			// it (it might be another tag, or movetext).
			tok = valTok
			continue
		}
		g.Tags = append(g.Tags, Tag{Name: tagName, Value: valTok.text})
		tok, err = p.nextToken()
		if err != nil {
			return nil, err
		}
	}

	// Movetext: tok already holds the first non-tag token.
	p.pushBack(tok)
	moves, prefixComments, result, terminator, err := p.parseMovetext(g, 0)
	if err != nil {
		return nil, err
	}
	g.Moves = moves
	g.PrefixComments = prefixComments
	g.Result = result
	g.EndLine = terminator.line

	if result == "" {
		g.errs = append(g.errs, &ParseError{Line: terminator.line, Msg: "missing result"})
	}

	// If the game ended at a tokTagName (recoverable end-of-game), push it
	// back so the next parseGame call sees it as the start of the next
	// game.
	if terminator.kind == tokTagName {
		p.pushBack(terminator)
	}

	return g, nil
}

// parseMovetext parses a sequence of moves (with attached comments, NAGs,
// and variations) until a terminating token is reached: tokResult,
// tokRAVEnd (only valid when depth > 0), tokTagName, or tokEOF. It returns
// the parsed moves, any comments seen before the first move, the result
// text (only meaningful for the top-level call; "" otherwise), and the
// terminating token so the caller can decide how to react.
func (p *parser) parseMovetext(g *Game, depth int) (moves []*Move, prefixComments []string, result string, terminator token, err error) {
	var current *Move
	sawMove := false
	pendingMoveNumber := 0
	haveMoveNumber := false

	for {
		tok, terr := p.nextToken()
		if terr != nil {
			return moves, prefixComments, result, token{}, terr
		}

		switch tok.kind {
		case tokEOF:
			return moves, prefixComments, result, tok, nil

		case tokTagName:
			// Recoverable end of game: push back handled by caller.
			return moves, prefixComments, result, tok, nil

		case tokResult:
			result = tok.text
			return moves, prefixComments, result, tok, nil

		case tokRAVEnd:
			if depth > 0 {
				return moves, prefixComments, result, tok, nil
			}
			// Unmatched RAV end at top level: recoverable error, skip.
			g.errs = append(g.errs, &ParseError{Line: tok.line, Msg: "unmatched )"})
			continue

		case tokRAVStart:
			if !sawMove || current == nil {
				// A variation with no preceding move to attach to. Parse it
				// (to stay synchronized with matching parens) but discard
				// it, recording a recoverable error.
				_, _, _, _, verr := p.parseMovetext(g, depth+1)
				if verr != nil {
					return moves, prefixComments, result, token{}, verr
				}
				g.errs = append(g.errs, &ParseError{Line: tok.line, Msg: "variation with no preceding move"})
				continue
			}
			subMoves, subPrefix, _, _, verr := p.parseMovetext(g, depth+1)
			if verr != nil {
				return moves, prefixComments, result, token{}, verr
			}
			current.Variations = append(current.Variations, &Variation{
				PrefixComments: subPrefix,
				Moves:          subMoves,
			})
			continue

		case tokComment:
			if !sawMove || current == nil {
				prefixComments = append(prefixComments, tok.text)
			} else {
				current.Comments = append(current.Comments, tok.text)
			}
			continue

		case tokNAG:
			if !sawMove || current == nil {
				// No preceding move to attach to: recoverable error,
				// matching the C oracle's silent-discard leniency but
				// keeping a record for diagnostics.
				g.errs = append(g.errs, &ParseError{Line: tok.line, Msg: "NAG with no preceding move"})
				continue
			}
			n, nerr := strconv.Atoi(tok.text)
			if nerr != nil {
				g.errs = append(g.errs, &ParseError{Line: tok.line, Msg: "unparsable NAG: $" + tok.text})
				continue
			}
			current.NAGs = append(current.NAGs, n)
			continue

		case tokMoveNumber:
			// A move number alone does not start a move; it is attached to
			// the next move token we see, skipping over any comments/NAGs
			// that appear in between (they attach normally in the meantime).
			pendingMoveNumber = parseMoveNumber(tok.text)
			haveMoveNumber = true
			continue

		case tokMove:
			mv := &Move{SAN: tok.text}
			if haveMoveNumber {
				mv.MoveNumber = pendingMoveNumber
				haveMoveNumber = false
			}
			moves = append(moves, mv)
			current = mv
			sawMove = true
			continue

		case tokString:
			// Stray string in movetext: recoverable error, skip.
			g.errs = append(g.errs, &ParseError{Line: tok.line, Msg: "unexpected string in movetext"})
			continue

		default:
			g.errs = append(g.errs, &ParseError{Line: tok.line, Msg: "unexpected token in movetext"})
			continue
		}
	}
}

func parseMoveNumber(text string) int {
	n, err := strconv.Atoi(text)
	if err != nil {
		return 0
	}
	return n
}

// ParseError is a recoverable error recorded on a Game while parsing.
type ParseError struct {
	Line int
	Msg  string
}

func (e *ParseError) Error() string {
	return "pgn: line " + strconv.Itoa(e.Line) + ": " + e.Msg
}
