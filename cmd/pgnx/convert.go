package main

import (
	"io"
	"os"
	"strings"

	"github.com/kyleboon/pgnx/output"
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

	addOutputFlags(cmd)
	addECOFlags(cmd)
	return cmd
}

// addOutputFlags registers the output-shaping flags shared by convert and
// filter, so that filter with no selection criteria behaves exactly like
// convert.
func addOutputFlags(cmd *cobra.Command) {
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
}

// forcedUCILineWidth is the line width pgn-extract's C oracle silently
// substitutes when -Wuci is given. argsfile.c's OUTPUT_FORMAT_ARGUMENT case
// calls set_output_line_length(5000) specifically for UCI ("@@@ Warning:
// arbitrary value." per its own comment) because UCI movetext is emitted as
// one token per ply with no natural wrap points; -w (LINE_WIDTH_ARGUMENT)
// overrides it only when given later on the command line. pgnx's convert
// mirrors this: it applies the same forced default for uci, unless -w/
// --line-width was explicitly passed (cobra's Changed flag stands in for
// "given later" since pgnx has no analogous ordering effect to replicate).
const forcedUCILineWidth = 5000

// outputOptions builds the output.Options for a command's Writer from the
// flags registered by addOutputFlags.
func outputOptions(cmd *cobra.Command) (output.Options, error) {
	flags := cmd.Flags()

	formatName, _ := flags.GetString("format")
	format, err := output.ParseFormat(formatName)
	if err != nil {
		return output.Options{}, err
	}

	lineWidth, _ := flags.GetInt("line-width")
	if format == output.UCI && !flags.Changed("line-width") {
		lineWidth = forcedUCILineWidth
	}
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

// openOutput opens the destination named by the string flag flagName
// (--output, or filter's --unmatched), truncating or appending per
// --append. An empty value, or "-", means cmd.OutOrStdout() (with a nil
// Closer).
func openOutput(cmd *cobra.Command, flagName string) (io.Writer, io.Closer, error) {
	path, _ := cmd.Flags().GetString(flagName)
	if path == "" || path == "-" {
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
	// convert is filter with no criteria: the pipeline decides everything
	// about a game and every game that is not Rejected goes to the output.
	r, err := newRun(cmd, nil, nil)
	if err != nil {
		return err
	}
	defer r.close()
	r.route = routeAll
	return r.exec(args)
}
