package main

import (
	"fmt"
	"io"
	"os"

	"github.com/kyleboon/pgnx/pgn"
	"github.com/spf13/cobra"
)

// openLogFile opens path for the --log flag. It always truncates/creates,
// matching typical log-file conventions for a fresh run.
func openLogFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
}

// forEachGame is the shared per-game iteration used by convert, validate,
// and info. It reads PGN text from each of files in turn (or from
// cmd.InOrStdin() when files is empty, labeled "stdin" in messages),
// parsing games one at a time and invoking fn for each with its 1-based
// game number (counted across all files) and source label.
//
// fn is expected to attempt whatever processing it needs (typically a
// replay) and return an error describing why the game failed, or nil on
// success. forEachGame logs per-game failures to w in the form
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
		total, succeeded, ioErr = scanGames(cmd, "stdin", cmd.InOrStdin(), w, ioW, total, succeeded, fn)
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
		total, succeeded, scanErr = scanGames(cmd, path, f, w, ioW, total, succeeded, fn)
		if scanErr != nil {
			ioErr = scanErr
		}
		f.Close()
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
// already produced for them) stand.
func scanGames(cmd *cobra.Command, src string, r io.Reader, w, ioW io.Writer, total, succeeded int, fn func(src string, n int, g *pgn.Game) error) (int, int, error) {
	opts := pgn.ParserOptions{}
	if nested, err := cmd.Flags().GetBool("nested-comments"); err == nil {
		opts.NestedComments = nested
	}
	scanner := pgn.NewScanner(r, opts)
	for scanner.Next() {
		total++
		g := scanner.Game()
		if err := fn(src, total, g); err != nil {
			fmt.Fprintf(w, "%s:%d: game %d: %v\n", src, g.StartLine, total, err)
			continue
		}
		succeeded++
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(ioW, "%s: %v\n", src, err)
		return total, succeeded, err
	}
	return total, succeeded, nil
}
