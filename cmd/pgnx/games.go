package main

import (
	"io"
	"iter"
	"os"

	"github.com/kyleboon/pgnx/pgn"
)

// openLogFile opens path for the --log flag. It always truncates/creates,
// matching typical log-file conventions for a fresh run.
func openLogFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
}

// Item is one step of the games iterator: a parsed game, or the failure
// of a source.
type Item struct {
	// Source is the file the item came from, or "stdin".
	Source string
	// Number is the game's 1-based number across all sources. On an Err
	// item it is the number of games yielded so far.
	Number int
	// Game is the parsed game; nil on an Err item.
	Game *pgn.Game
	// Skipped are the tokens the parser discarded from Game (a lone "-",
	// a bare piece letter, ...). The game is emitted regardless, but the
	// report counts it as failed.
	Skipped []error
	// Err is a source's failure: the file could not be opened, or the
	// scan hit a fatal error (an unterminated comment, say) after which
	// nothing more is read from it. The walk goes on with the next
	// source.
	Err error
}

// games walks files in order (or stdin, labelled "stdin", when files is
// empty) and yields every game, numbered across all files, followed by
// an Err item for each source that failed. It reads nothing beyond the
// game it is yielding (ADR 0003), so breaking out of the loop stops
// reading at once, across all remaining files. opts configures the
// parser; the iterator itself knows nothing about flags.
func games(files []string, stdin io.Reader, opts pgn.ParserOptions) iter.Seq[Item] {
	return func(yield func(Item) bool) {
		n := 0
		if len(files) == 0 {
			scanGames("stdin", stdin, opts, &n, yield)
			return
		}
		for _, path := range files {
			f, err := os.Open(path)
			if err != nil {
				if !yield(Item{Source: path, Number: n, Err: err}) {
					return
				}
				continue
			}
			more := scanGames(path, f, opts, &n, yield)
			f.Close()
			if !more {
				return
			}
		}
	}
}

// scanGames yields the games of r, counting on n, then an Err item if the
// scan ended with a fatal error. It reports whether the consumer wants
// more.
func scanGames(src string, r io.Reader, opts pgn.ParserOptions, n *int, yield func(Item) bool) bool {
	scanner := pgn.NewScanner(r, opts)
	for scanner.Next() {
		*n++
		g := scanner.Game()
		if !yield(Item{Source: src, Number: *n, Game: g, Skipped: g.SkippedTokens()}) {
			return false
		}
	}
	if err := scanner.Err(); err != nil {
		return yield(Item{Source: src, Number: *n, Err: err})
	}
	return true
}
