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
	opts, err := outputOptions(cmd)
	if err != nil {
		return err
	}
	ecoSet, err := ecoOptions(cmd)
	if err != nil {
		return err
	}
	keepBroken, _ := cmd.Flags().GetBool("keep-broken")
	quiet, _ := cmd.Flags().GetBool("quiet")

	logW, logCloser, err := logWriter(cmd)
	if err != nil {
		return err
	}
	if logCloser != nil {
		defer logCloser.Close()
	}
	if !quiet {
		ecoPath, _ := cmd.Flags().GetString("eco-file")
		ecoSet.logSkipped(logW, ecoPath)
	}

	// One registry shared by every writer, fed every parsed game so that
	// unrecognised tags are ordered as the C oracle orders them (run-wide
	// first-seen), even when a game is skipped.
	opts.Tags = output.NewTagRegistry()
	if opts.SevenTagRoster && ecoSet.classifying() {
		opts.RosterExtras = ecoRosterExtras
	}
	dest, err := openDestination(cmd, "output", opts, ecoSet.split)
	if err != nil {
		return err
	}
	defer dest.Close()

	total, succeeded, ioErr := forEachGame(cmd, args, logW, logW, func(src string, n int, g *pgn.Game) error {
		opts.Tags.Observe(g)
		r, replayErr := replay.Game(g, replay.Options{})
		if replayErr != nil && !keepBroken {
			return replayErr
		}
		ecoSet.classify(g, r)
		if err := writeGameTo(dest, g, r, replayErr); err != nil {
			return err
		}
		return replayErr
	})

	if !quiet {
		fmt.Fprintf(logW, "%d game%s matched out of %d.\n", succeeded, plural(succeeded), total)
	}

	if ioErr != nil || succeeded < total {
		return errProblems
	}
	return nil
}

// openDestination builds the writer selector for games: with --eco-split
// a per-ECO-prefix set of files in the current directory, otherwise the
// file named by the string flag flagName (or stdout). The returned
// Closer closes whatever files were opened.
func openDestination(cmd *cobra.Command, flagName string, opts output.Options, splitLevel int) (gameWriters, error) {
	sourceOpts := output.Options{Format: output.Source, LineWidth: opts.LineWidth, Tags: opts.Tags}
	if splitLevel > 0 {
		return newECOSplitWriters(splitLevel, opts, sourceOpts), nil
	}
	out, closer, err := openOutput(cmd, flagName)
	if err != nil {
		return nil, err
	}
	return &closingWriters{
		fixedWriters: fixedWriters{w: output.NewWriter(out, opts), sourceW: output.NewWriter(out, sourceOpts)},
		closer:       closer,
	}, nil
}

// closingWriters is a fixedWriters pair over a file that must be closed
// at the end of the run (nil for stdout).
type closingWriters struct {
	fixedWriters
	closer io.Closer
}

func (c *closingWriters) Close() error {
	if c.closer == nil {
		return nil
	}
	return c.closer.Close()
}

// plural returns "" when n == 1, else "s", matching the C oracle's
// "%lu game%s matched out of %lu.\n" wording (main.c).
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
