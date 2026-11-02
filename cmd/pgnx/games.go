package main

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"os"

	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/pipeline"
	"github.com/spf13/cobra"
)

// errStop may be returned by a forEachGame callback to end iteration
// cleanly after the current game: it is not logged and the game is
// counted as succeeded. To stop after a game that also failed, wrap the
// failure with stopAfter so it is still logged and counted.
var errStop = errors.New("stop")

// stopError carries a per-game failure that should also end iteration.
type stopError struct{ err error }

func (e *stopError) Error() string        { return e.err.Error() }
func (e *stopError) Unwrap() error        { return e.err }
func (e *stopError) Is(target error) bool { return target == errStop }

// stopAfter returns an error that scanGames treats as "log/count err as
// usual, then stop": errStop itself when err is nil.
func stopAfter(err error) error {
	if err == nil {
		return errStop
	}
	return &stopError{err: err}
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

// openLogFile opens path for the --log flag. It always truncates/creates,
// matching typical log-file conventions for a fresh run.
func openLogFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
}

// forEachGame is the shared per-game iteration used by every command
// (run.exec for convert and filter; validate and info call it directly).
// It reads PGN text from each of files in turn (or from
// cmd.InOrStdin() when files is empty, labeled "stdin" in messages),
// parsing games one at a time and invoking fn for each with its 1-based
// game number (counted across all files) and source label.
//
// fn is expected to attempt whatever processing it needs (typically a
// replay) and return an error describing why the game failed, or nil on
// success; returning errStop (or stopAfter(err)) ends iteration after
// that game, across all remaining files. forEachGame logs per-game failures to w in the form
// "<file>:<line>: game <n>: <error>", and I/O errors opening a file to
// ioW as "<file>: <error>", and returns the total number of games seen
// and the number that succeeded. The two writers are separate so a caller
// can suppress or redirect per-game findings (as validate --summary does)
// without also silencing file-open errors, which must always stay
// visible; callers with no such split simply pass the same writer twice.
// It does not itself decide the process exit code; callers compare
// succeeded to total.
//
// ioErr retains only the last file-open error (each one is already logged
// to ioW as it occurs); callers treat any non-nil ioErr as "problems
// seen".
func forEachGame(cmd *cobra.Command, files []string, w, ioW io.Writer, fn func(src string, n int, g *pgn.Game) error) (total, succeeded int, ioErr error) {
	if len(files) == 0 {
		total, succeeded, ioErr, _ = oldScanGames(cmd, "stdin", cmd.InOrStdin(), w, ioW, total, succeeded, fn)
		return total, succeeded, ioErr
	}

	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(ioW, "%s: %v\n", path, err)
			ioErr = err
			continue
		}
		var scanErr error
		var stopped bool
		total, succeeded, scanErr, stopped = oldScanGames(cmd, path, f, w, ioW, total, succeeded, fn)
		if scanErr != nil {
			ioErr = scanErr
		}
		f.Close()
		if stopped {
			break
		}
	}
	return total, succeeded, ioErr
}

// scanGames scans all games out of r (sourced from src), invoking fn for
// each and logging failures to w, accumulating onto the running total and
// succeeded counters. If the scan stops early because of a fatal scanner
// error (a lexer/parser error such as an unterminated comment, as opposed
// to an ordinary end of input), that error is written to ioW as
// "<src>: <error>" and also returned, so callers treat it the same as a
// file-open failure: visible on the I/O-error writer and sufficient to
// fail the run, even though any games already scanned (and any output
// already produced for them) stand. stopped reports that fn asked to
// stop (errStop); no further games are read from r in that case.
func oldScanGames(cmd *cobra.Command, src string, r io.Reader, w, ioW io.Writer, total, succeeded int, fn func(src string, n int, g *pgn.Game) error) (_, _ int, err error, stopped bool) {
	// Every command skips tags-only games (a tag section with no
	// movetext), which pgn-extract never emits, so counts and numbering
	// match the oracle (ADR 0002, CONFORMANCE.md).
	opts := pgn.ParserOptions{SkipTagsOnly: true}
	if nested, err := cmd.Flags().GetBool("nested-comments"); err == nil {
		opts.NestedComments = nested
	}
	scanner := pgn.NewScanner(r, opts)
	for scanner.Next() {
		total++
		g := scanner.Game()
		err := fn(src, total, g)
		// Tokens the parser discarded (a lone "-", a bare piece letter,
		// ...) are reported like the C oracle's warnings, and the game is
		// emitted regardless; but it still counts as a problem, so the run
		// exits 2 (the C tool exits 0 here).
		skipped := g.SkippedTokens()
		for _, serr := range skipped {
			fmt.Fprintf(w, "%s:%d: game %d: warning: %v\n", src, g.StartLine, total, serr)
		}
		if errors.Is(err, errStop) {
			stopped = true
			var se *stopError
			if errors.As(err, &se) {
				err = se.err
			} else {
				err = nil
			}
		}
		if err != nil {
			fmt.Fprintf(w, "%s:%d: game %d: %v\n", src, g.StartLine, total, err)
		} else if len(skipped) == 0 {
			succeeded++
		}
		if stopped {
			return total, succeeded, nil, true
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(ioW, "%s: %v\n", src, err)
		return total, succeeded, err, false
	}
	return total, succeeded, nil, false
}

// problems folds a pipeline outcome into the forEachGame callback's
// error-as-control protocol: the reason a game is Broken or Rejected,
// and any warnings, all of which make the game count as a failure. nil
// when the game is OK with no warnings.
func problems(o pipeline.Outcome) error {
	if o.Reason == nil && len(o.Warnings) == 0 {
		return nil
	}
	return errors.Join(append([]error{o.Reason}, o.Warnings...)...)
}
