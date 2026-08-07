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
// success. forEachGame logs failures (and I/O errors opening a file) to w
// in the form "<file>:<line>: game <n>: <error>", and returns the total
// number of games seen and the number that succeeded. It does not itself
// decide the process exit code; callers compare succeeded to total.
//
// ioErr retains only the last file-open error (each one is already logged
// to w as it occurs); callers treat any non-nil ioErr as "problems seen".
func forEachGame(cmd *cobra.Command, files []string, w io.Writer, fn func(src string, n int, g *pgn.Game) error) (total, succeeded int, ioErr error) {
	if len(files) == 0 {
		total, succeeded = scanGames(cmd, "stdin", cmd.InOrStdin(), w, total, succeeded, fn)
		return total, succeeded, nil
	}

	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(w, "%s: %v\n", path, err)
			ioErr = err
			continue
		}
		total, succeeded = scanGames(cmd, path, f, w, total, succeeded, fn)
		f.Close()
	}
	return total, succeeded, ioErr
}

// scanGames scans all games out of r (sourced from src), invoking fn for
// each and logging failures to w, accumulating onto the running total and
// succeeded counters.
func scanGames(cmd *cobra.Command, src string, r io.Reader, w io.Writer, total, succeeded int, fn func(src string, n int, g *pgn.Game) error) (int, int) {
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
	return total, succeeded
}
