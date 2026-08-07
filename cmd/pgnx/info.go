package main

import (
	"fmt"

	"github.com/kyleboon/pgnx/pgn"
	"github.com/spf13/cobra"
)

// newInfoCmd builds the "pgnx info" subcommand: it reports aggregate
// statistics (game count, ply count, result breakdown) over its input
// (files, or stdin when none are given). Unlike validate, info never
// replays games — it only needs what the parser already recorded (the
// main-line move list and the result token), so a game with illegal moves
// still contributes to the counts instead of being reported as broken.
func newInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info [file...]",
		Short: "Summarize game count, ply count, and results",
		RunE:  runInfo,
	}

	cmd.Flags().Bool("nested-comments", false, "allow { to nest inside a comment")

	return cmd
}

func runInfo(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	logW, logCloser, err := logWriter(cmd)
	if err != nil {
		return err
	}
	if logCloser != nil {
		defer logCloser.Close()
	}

	var plies int
	var win, loss, draw, unknown int

	total, _, ioErr := forEachGame(cmd, args, logW, func(src string, n int, g *pgn.Game) error {
		plies += len(g.Moves)
		switch g.Result {
		case "1-0":
			win++
		case "0-1":
			loss++
		case "1/2-1/2":
			draw++
		default:
			unknown++
		}
		return nil
	})

	avg := 0.0
	if total > 0 {
		avg = float64(plies) / float64(total)
	}

	fmt.Fprintf(out, "games:   %d\n", total)
	fmt.Fprintf(out, "plies:   %d (avg %.1f)\n", plies, avg)
	fmt.Fprintf(out, "results: 1-0 %d, 0-1 %d, 1/2-1/2 %d, * %d\n", win, loss, draw, unknown)

	if ioErr != nil {
		return errProblems
	}
	return nil
}
