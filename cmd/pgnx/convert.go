package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kyleboon/pgnx/output"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
	"github.com/spf13/cobra"
)

// newConvertCmd builds the "pgnx convert" subcommand: it reads PGN games
// from its input (files, or stdin when none are given), replays each one
// to validate it and gather the detail needed by non-Source output
// formats, and writes the successfully-replayed games back out in the
// requested format.
func newConvertCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "convert [file...]",
		Short: "Convert PGN games between move-notation formats",
		RunE:  runConvert,
	}

	flags := cmd.Flags()
	flags.StringP("format", "f", "san", "output format: san, source, lalg, halg, elalg, xlalg, uci, epd, fen")
	flags.StringP("output", "o", "", "write output to FILE instead of stdout")
	flags.Bool("append", false, "append to the --output file instead of truncating it")
	flags.IntP("line-width", "w", 75, "word-wrap movetext at this column")
	flags.Bool("seven-tag-roster", false, "emit only the seven mandatory header tags")
	flags.String("roster", "", "comma-separated explicit list of header tags to emit")
	flags.Bool("no-comments", false, "omit comments")
	flags.Bool("no-nags", false, "omit NAGs")
	flags.Bool("no-variations", false, "omit variations")
	flags.Bool("no-move-numbers", false, "omit move numbers")
	flags.Bool("no-results", false, "omit the trailing result token")
	flags.Bool("keep-broken", false, "emit games that fail replay, in Source format, instead of skipping them")
	flags.Bool("nested-comments", false, "allow { to nest inside a comment")

	return cmd
}

// convertOptions builds the output.Options that runConvert's Writer uses,
// from cmd's convert-specific flags.
func convertOptions(cmd *cobra.Command) (output.Options, error) {
	flags := cmd.Flags()

	formatName, _ := flags.GetString("format")
	format, err := output.ParseFormat(formatName)
	if err != nil {
		return output.Options{}, err
	}

	lineWidth, _ := flags.GetInt("line-width")
	sevenTagRoster, _ := flags.GetBool("seven-tag-roster")
	rosterCSV, _ := flags.GetString("roster")
	noComments, _ := flags.GetBool("no-comments")
	noNAGs, _ := flags.GetBool("no-nags")
	noVariations, _ := flags.GetBool("no-variations")
	noMoveNumbers, _ := flags.GetBool("no-move-numbers")
	noResults, _ := flags.GetBool("no-results")

	var roster []string
	if rosterCSV != "" {
		for _, name := range strings.Split(rosterCSV, ",") {
			roster = append(roster, strings.TrimSpace(name))
		}
	}

	return output.Options{
		Format:         format,
		LineWidth:      lineWidth,
		SevenTagRoster: sevenTagRoster,
		Roster:         roster,
		NoComments:     noComments,
		NoNAGs:         noNAGs,
		NoVariations:   noVariations,
		NoMoveNumbers:  noMoveNumbers,
		NoResults:      noResults,
	}, nil
}

// convertOutput opens the destination for rendered games per the --output
// and --append flags, returning cmd.OutOrStdout() (with a no-op Closer)
// when --output was not given.
func convertOutput(cmd *cobra.Command) (io.Writer, io.Closer, error) {
	path, _ := cmd.Flags().GetString("output")
	if path == "" {
		return cmd.OutOrStdout(), nil, nil
	}
	append_, _ := cmd.Flags().GetBool("append")
	flags := os.O_WRONLY | os.O_CREATE
	if append_ {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return f, f, nil
}

func runConvert(cmd *cobra.Command, args []string) error {
	opts, err := convertOptions(cmd)
	if err != nil {
		return err
	}
	keepBroken, _ := cmd.Flags().GetBool("keep-broken")
	quiet, _ := cmd.Flags().GetBool("quiet")

	out, outCloser, err := convertOutput(cmd)
	if err != nil {
		return err
	}
	if outCloser != nil {
		defer outCloser.Close()
	}

	logW, logCloser, err := logWriter(cmd)
	if err != nil {
		return err
	}
	if logCloser != nil {
		defer logCloser.Close()
	}

	w := output.NewWriter(out, opts)
	sourceWriter := output.NewWriter(out, output.Options{Format: output.Source, LineWidth: opts.LineWidth})

	total, succeeded, ioErr := forEachGame(cmd, args, logW, func(src string, n int, g *pgn.Game) error {
		r, replayErr := replay.Game(g, replay.Options{})
		if replayErr != nil {
			if keepBroken {
				if err := sourceWriter.WriteGame(g, nil); err != nil {
					return err
				}
			}
			return replayErr
		}
		return w.WriteGame(g, r)
	})

	if !quiet {
		fmt.Fprintf(logW, "%d game%s matched out of %d.\n", succeeded, plural(succeeded), total)
	}

	if ioErr != nil || succeeded < total {
		return errProblems
	}
	return nil
}

// plural returns "" when n == 1, else "s", matching the C oracle's
// "%lu game%s matched out of %lu.\n" wording (main.c).
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
