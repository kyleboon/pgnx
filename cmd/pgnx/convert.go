package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kyleboon/pgnx/output"
	"github.com/kyleboon/pgnx/transform"
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
	flags.Bool("allow-null-moves", false, "accept the null move -- in the main line; always accepted in variations (--allownullmoves)")
	flags.Bool("nested-comments", false, "allow { to nest inside a comment")

	// Output options (pgn-extract's output.c): how the game is rendered.
	flags.Bool("no-tags", false, "omit the tag section (--notags; conflicts with --seven-tag-roster and --roster)")
	flags.Bool("no-checks", false, "omit + and # from emitted moves (--nochecks)")
	flags.Bool("comment-lines", false, "put each comment on a line of its own (--commentlines)")
	flags.Bool("fen-comments", false, "follow every move with a { FEN } comment of the position reached (--fencomments)")
	flags.Bool("hash-comments", false, "follow every move with a { hash } comment of the position reached, in hex (--hashcomments)")
	flags.Bool("evaluation", false, "follow every move with a { value } comment holding a Shannon evaluation of the position (--evaluation)")
	flags.Bool("final-fen", false, "follow the last move with a { \"FEN\" } comment of the final position (-F)")
	flags.Int("ply-limit", 0, "emit at most N plies of each game; a cut game ends with * (--plylimit N)")

	// Transforms (pgn-extract's apply.c/format_game rewrites): what the
	// emitted game holds.
	flags.Int("drop-ply", 0, "drop the first N plies and start the game from the position reached; negative N keeps the last -N plies (--dropply N)")
	flags.String("drop-before", "", "drop every ply up to and including the move whose comment is exactly TEXT (--dropbefore TEXT)")
	flags.Bool("ply-count", false, "add a PlyCount tag (--plycount)")
	flags.Bool("total-ply-count", false, "add a TotalPlyCount tag counting variations too (--totalplycount)")
	flags.Bool("add-hash-code", false, "add a HashCode tag (--addhashcode)")
	flags.String("add-elo-tags", "", "add WhiteElo/BlackElo tags from the tab-separated name/rating FILE (--addelotags FILE)")
	flags.String("add-fide-id-tags", "", "add WhiteFideId/BlackFideId tags from the tab-separated name/ID FILE (--addfideidtags FILE)")
	flags.String("line-numbers", "", "prepend a { MARKER:start:end } comment giving each game's source lines (--linenumbers MARKER)")
	flags.Bool("split-variants", false, "emit each variation as a game of its own after the main line, stripped of variations (--splitvariants; conflicts with --no-variations)")
	flags.Int("split-depth", 0, "with --split-variants, follow variations only N levels deep; 0 is unlimited (--splitvariants N)")
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
	noTags, _ := flags.GetBool("no-tags")
	if noTags && (sevenTagRoster || rosterCSV != "") {
		// argsfile.c: "--notags clashes with another roster-related argument."
		return output.Options{}, errors.New("--no-tags conflicts with --seven-tag-roster and --roster")
	}
	noChecks, _ := flags.GetBool("no-checks")
	commentLines, _ := flags.GetBool("comment-lines")
	fenComments, _ := flags.GetBool("fen-comments")
	hashComments, _ := flags.GetBool("hash-comments")
	evaluation, _ := flags.GetBool("evaluation")
	finalFEN, _ := flags.GetBool("final-fen")
	hasPlyLimit := flags.Changed("ply-limit")
	plyLimit, _ := flags.GetInt("ply-limit")
	if hasPlyLimit && plyLimit < 0 {
		return output.Options{}, errors.New("--ply-limit requires a number greater than or equal to zero")
	}

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
		NoTags:         noTags,
		NoChecks:       noChecks,
		CommentLines:   commentLines,
		FENComments:    fenComments,
		HashComments:   hashComments,
		Evaluation:     evaluation,
		FinalFEN:       finalFEN,
		PlyLimit:       plyLimit,
		HasPlyLimit:    hasPlyLimit,
	}, nil
}

// transformOptions builds the transform.Options for a command's pipeline
// from the flags registered by addOutputFlags, loading the player files.
// Every error is a usage error (exit 1).
func transformOptions(cmd *cobra.Command) (*transform.Options, error) {
	flags := cmd.Flags()
	var opts transform.Options
	opts.DropPlies, _ = flags.GetInt("drop-ply")
	opts.DropBefore, _ = flags.GetString("drop-before")
	opts.PlyCount, _ = flags.GetBool("ply-count")
	opts.TotalPlyCount, _ = flags.GetBool("total-ply-count")
	opts.HashCode, _ = flags.GetBool("add-hash-code")
	opts.LineNumbers, _ = flags.GetString("line-numbers")
	// -Wuci forces variations off in the C tool (argsfile.c), and
	// TotalPlyCount follows keep_variations.
	noVariations, _ := flags.GetBool("no-variations")
	formatName, _ := flags.GetString("format")
	opts.CountVariations = !noVariations && formatName != "uci"

	for _, f := range []struct {
		flag string
		dest *transform.PlayerInfo
	}{{"add-elo-tags", &opts.Ratings}, {"add-fide-id-tags", &opts.IDs}} {
		path, _ := flags.GetString(f.flag)
		if path == "" {
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("--%s: %w", f.flag, err)
		}
		info, err := transform.LoadPlayerInfo(file)
		file.Close()
		if err != nil {
			return nil, fmt.Errorf("--%s %s: %w", f.flag, path, err)
		}
		*f.dest = info
	}
	return &opts, nil
}

// splitOptions reads --split-variants and --split-depth. Their conflicts
// are usage errors (exit 1), as -V with --splitvariants is in the C tool.
// UCI output drops variations (-Wuci turns keep_variations off in
// argsfile.c), and the C tool splits only with them kept, so --format uci
// does not split.
func splitOptions(cmd *cobra.Command) (split bool, depth int, err error) {
	flags := cmd.Flags()
	split, _ = flags.GetBool("split-variants")
	depth, _ = flags.GetInt("split-depth")
	noVariations, _ := flags.GetBool("no-variations")
	switch {
	case split && noVariations:
		// argsfile.c: "-V clashes with the --splitvariants flag."
		return false, 0, errors.New("--split-variants conflicts with --no-variations")
	case flags.Changed("split-depth") && !split:
		return false, 0, errors.New("--split-depth requires --split-variants")
	case depth < 0:
		return false, 0, errors.New("--split-depth requires a number greater than or equal to zero")
	}
	formatName, _ := flags.GetString("format")
	if format, err := output.ParseFormat(formatName); err == nil && format == output.UCI {
		split = false
	}
	return split, depth, nil
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
	// about a game and every game that is not Rejected goes to the output
	// (newRun's route, routeAll).
	r, err := newRun(cmd, nil)
	if err != nil {
		return err
	}
	return r.finish(r.exec(args))
}
