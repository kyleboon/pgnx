// Package pgn implements a lexer, parser, and scanner for Portable Game
// Notation (PGN) chess game text, turning a stream of PGN source into a
// sequence of Game values that other packages (replay, output) can
// consume.
package pgn

import "io"

// Scanner reads a stream of PGN text and yields Games one at a time,
// mirroring the usage pattern of bufio.Scanner.
type Scanner struct {
	p    *parser
	game *Game
	err  error
	done bool
}

// NewScanner creates a Scanner reading games from r.
func NewScanner(r io.Reader, opts ParserOptions) *Scanner {
	return &Scanner{p: newParser(newLexer(r, opts))}
}

// Next advances the Scanner to the next game. It returns false when there
// are no more games, either because the input is exhausted or because a
// fatal error occurred (check Err in that case).
func (s *Scanner) Next() bool {
	if s.done {
		return false
	}
	g, err := s.p.parseGame()
	if err != nil {
		s.err = err
		s.done = true
		s.game = nil
		return false
	}
	if g == nil {
		s.done = true
		s.game = nil
		return false
	}
	s.game = g
	return true
}

// Game returns the most recently parsed game, i.e. the one produced by the
// last call to Next that returned true.
func (s *Scanner) Game() *Game {
	return s.game
}

// Err returns the first fatal error encountered, if any. Recoverable
// per-game parsing problems are not fatal; see Game.ParseErrors.
func (s *Scanner) Err() error {
	return s.err
}
