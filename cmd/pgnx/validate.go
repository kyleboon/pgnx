package main

import (
	"io"

	"github.com/spf13/cobra"
)

// newValidateCmd builds the "pgnx validate" subcommand: it reads PGN games
// from its input (files, or stdin when none are given), normalises and
// replays each one, and reports which games fail: a game is valid when
// convert would emit it without a failure. Unlike convert, validate's per-game
// findings are a primary output rather than incidental logging, so they go
// to stdout; --summary suppresses them in favor of a single tally line.
func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [file...]",
		Short: "Check PGN games for illegal or unparsable moves",
		RunE:  runValidate,
	}

	cmd.Flags().Bool("summary", false, "print only the final \"<valid> of <total> games valid\" line")
	cmd.Flags().Bool("nested-comments", false, "allow { to nest inside a comment")
	cmd.Flags().Bool("allow-null-moves", false, "accept the null move -- in the main line; always accepted in variations (--allownullmoves)")

	return cmd
}

func runValidate(cmd *cobra.Command, args []string) error {
	summary, _ := cmd.Flags().GetBool("summary")
	out := cmd.OutOrStdout()

	// Per-game findings are a primary output for validate, so they go to
	// stdout (out) rather than the shared log writer used by convert. With
	// --summary, the report's per-game lines are unwanted: every game is
	// still validated (for an accurate tally), but the findings themselves
	// are discarded in favor of the single tally line printed below.
	findings := out
	if summary {
		findings = io.Discard
	}

	// I/O errors (unreadable input files) are not findings: they must stay
	// visible even under --summary, and they belong on the log writer
	// (stderr, or --log) so stdout carries only findings and the tally.
	logW, logCloser, err := logWriter(cmd)
	if err != nil {
		return err
	}
	if logCloser != nil {
		defer logCloser.Close()
	}

	// "Valid" means convert would emit the game with no failure: the same
	// per-game sequence, routed nowhere, so a Rejected game or a warning
	// is a finding.
	allowNullMoves, _ := cmd.Flags().GetBool("allow-null-moves")
	r := newReportOnlyRun(allowNullMoves)
	rep := newRunReport(findings, logW)
	rep.each(games(args, cmd.InOrStdin(), parserOptions(cmd)), r.game)

	if summary {
		rep.validSummary(out)
	}
	return rep.exit()
}
