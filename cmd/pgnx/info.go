package main

import (
	"fmt"

	"github.com/kyleboon/pgnx/pgn"
	"github.com/spf13/cobra"
)

// rosterTagNames is the seven mandatory PGN header tags (the Seven Tag
// Roster), in their canonical order, used by "pgnx info" to report how
// many games in the input carry each one.
var rosterTagNames = []string{"Event", "Site", "Date", "Round", "White", "Black", "Result"}

// newInfoCmd builds the "pgnx info" subcommand: it reports aggregate
// statistics (game count, ply count, result breakdown, and seven-tag-
// roster coverage) over its input (files, or stdin when none are given).
// Unlike validate, info never replays games — it only needs what the
// parser already recorded (the main-line move list, the result token, and
// the tags), so a game with illegal moves still contributes to the counts
// instead of being reported as broken.
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
	tagCounts := make(map[string]int, len(rosterTagNames))

	total, _, ioErr := forEachGame(cmd, args, logW, logW, func(src string, n int, g *pgn.Game) error {
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
		for _, name := range rosterTagNames {
			if _, found := g.Tag(name); found {
				tagCounts[name]++
			}
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
	fmt.Fprint(out, "tags:    ")
	for i, name := range rosterTagNames {
		if i > 0 {
			fmt.Fprint(out, ", ")
		}
		fmt.Fprintf(out, "%s %d", name, tagCounts[name])
	}
	fmt.Fprintln(out)

	if ioErr != nil {
		return errProblems
	}
	return nil
}
