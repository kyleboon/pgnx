package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/kyleboon/pgnx/filter"
	"github.com/kyleboon/pgnx/output"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
	"github.com/spf13/cobra"
)

// newFilterCmd builds the "pgnx filter" subcommand: convert's output
// flags plus game-selection criteria. Games that satisfy every criterion
// are written to --output; the rest can be routed to --unmatched.
func newFilterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "filter [file...]",
		Short: "Select games by tag, length, end state and material criteria",
		Long: `Select games from PGN input by tag criteria, length, end state, material
balance and result consistency. Output flags are the same as convert, so
filter with no criteria is equivalent to convert.

Tag criteria use pgn-extract's -t line grammar: Tag [op] "value", where op
is one of < <= <> > >= = =~ (regular expression) and is optional (prefix
match). Criteria on the same tag are alternatives; criteria on different
tags must all hold. Player, Elo and EloDiff are pseudo-tags covering either
colour.`,
		RunE: runFilter,
	}
	addOutputFlags(cmd)

	flags := cmd.Flags()
	flags.StringArray("tag", nil, "tag criterion in -t grammar, e.g. 'WhiteElo >= \"2500\"' (repeatable)")
	flags.StringArray("tag-file", nil, "file of tag criteria, one per line (repeatable)")
	flags.StringArray("player", nil, "player name prefix, either colour (-Tp; repeatable)")
	flags.StringArray("white", nil, "White player name prefix (-Tw; repeatable)")
	flags.StringArray("black", nil, "Black player name prefix (-Tb; repeatable)")
	flags.StringArray("date", nil, "Date prefix, e.g. 1970 or 1970.05 (-Td; repeatable)")
	flags.StringArray("result", nil, "Result prefix, e.g. 1-0 (-Tr; repeatable)")
	flags.StringArray("eco", nil, "ECO code prefix, e.g. B9 (-Te; repeatable)")
	flags.Bool("tag-substring", false, "match plain criteria anywhere in the tag value, not just at the start")
	flags.Bool("soundex", false, "match player, event, site and annotator names by soundex")
	flags.Bool("higher-rated-winner", false, "only games won by the higher-rated player")
	flags.Bool("lower-rated-winner", false, "only games won by the lower-rated player")
	flags.Bool("no-setup-tags", false, "exclude games with a SetUp tag")
	flags.Bool("only-setup-tags", false, "only games with a SetUp tag")

	flags.Int("min-moves", 0, "at least N full moves (-bl)")
	flags.Int("max-moves", 0, "at most N full moves (-bu)")
	flags.Int("moves", 0, "exactly N full moves (-b)")
	flags.Int("min-ply", 0, "at least N plies (-pl)")
	flags.Int("max-ply", 0, "at most N plies (-pu)")
	flags.Int("ply", 0, "exactly N plies (-p)")

	flags.Bool("checkmate", false, "only games ending in checkmate")
	flags.Bool("stalemate", false, "only games ending in stalemate")
	flags.Bool("insufficient", false, "only games ending with insufficient mating material")
	flags.Bool("underpromotion", false, "only games containing a promotion to other than a queen")
	flags.Bool("commented", false, "only games containing at least one comment")
	flags.Bool("fifty", false, "only games where the fifty-move rule became available")
	flags.Bool("seventy-five", false, "only games where the seventy-five-move rule became available")
	flags.Int("repetition", 0, "only games containing an N-fold repetition (--repetition is 3; --repetition=5)")
	flags.Lookup("repetition").NoOptDefVal = "3"
	flags.Int("piece-count", 0, "only games reaching a position with exactly N pieces")
	flags.StringArray("material", nil, "material balance pattern, either colour first (-z; repeatable)")
	flags.StringArray("material-file", nil, "file of material patterns, either colour first (-z; repeatable)")
	flags.StringArray("white-material", nil, "material balance pattern, White's set first (-y; repeatable)")
	flags.StringArray("white-material-file", nil, "file of material patterns, White's set first (-y; repeatable)")
	flags.String("mark-matches", "", "insert { TEXT } after each move matching a fifty/seventy-five, repetition, material or piece-count criterion (TEXT=FEN inserts the position)")
	flags.Bool("add-match-tag", false, "add a MaterialMatch tag naming the colour that matched a material pattern")

	flags.String("unmatched", "", "write games that do not match to FILE (- for stdout)")
	flags.Bool("suppress-matched", false, "do not write matched games (useful with --unmatched)")
	flags.String("select-only", "", "emit only matched games numbered in this list, e.g. 2,5:7")
	flags.String("skip-matching", "", "do not emit matched games numbered in this list, e.g. 1,2")
	flags.Int("stop-after", 0, "stop after N games have matched")
	flags.Int("first-game", 0, "ignore games before game number N")
	flags.Int("game-limit", 0, "stop after processing game number N")

	flags.Bool("no-bad-results", false, "reject games whose Result tag disagrees with the terminating result")
	flags.Bool("fix-result-tags", false, "correct the Result tag from the final position and terminating result")

	return cmd
}

// filterOptions translates the filter flags into filter.Options and
// filter.SelectorOptions, validating combinations. Every error it returns
// is a usage error (exit 1).
func filterOptions(cmd *cobra.Command) (*filter.Options, filter.SelectorOptions, error) {
	flags := cmd.Flags()
	o := &filter.Options{}
	var sel filter.SelectorOptions

	getBool := func(name string) bool { v, _ := flags.GetBool(name); return v }
	getInt := func(name string) int { v, _ := flags.GetInt(name); return v }
	getStr := func(name string) string { v, _ := flags.GetString(name); return v }
	getList := func(name string) []string { v, _ := flags.GetStringArray(name); return v }

	m := &filter.TagMatcher{Substring: getBool("tag-substring"), Soundex: getBool("soundex")}
	for _, path := range getList("tag-file") {
		f, err := os.Open(path)
		if err != nil {
			return nil, sel, err
		}
		cs, err := filter.ReadCriteria(f)
		f.Close()
		if err != nil {
			return nil, sel, fmt.Errorf("%s: %w", path, err)
		}
		for _, c := range cs {
			m.Add(c)
		}
	}
	for _, expr := range getList("tag") {
		c, err := filter.ParseCriterion(expr)
		if err != nil {
			return nil, sel, fmt.Errorf("--tag: %w", err)
		}
		m.Add(c)
	}
	for flag, tag := range map[string]string{
		"player": "Player", "white": "White", "black": "Black",
		"date": "Date", "result": "Result", "eco": "ECO",
	} {
		for _, v := range getList(flag) {
			m.AddPrefix(tag, v)
		}
	}
	if !m.Empty() {
		o.Tags = m
	}

	o.HigherRatedWinner = getBool("higher-rated-winner")
	o.LowerRatedWinner = getBool("lower-rated-winner")
	switch {
	case getBool("no-setup-tags") && getBool("only-setup-tags"):
		return nil, sel, errors.New("--no-setup-tags and --only-setup-tags are mutually exclusive")
	case getBool("no-setup-tags"):
		o.Setup = filter.NoSetup
	case getBool("only-setup-tags"):
		o.Setup = filter.OnlySetup
	}

	for _, name := range []string{"min-moves", "max-moves", "moves", "min-ply", "max-ply", "ply"} {
		if flags.Changed(name) && getInt(name) <= 0 {
			return nil, sel, fmt.Errorf("--%s must be at least 1", name)
		}
	}
	if flags.Changed("min-moves") {
		o.Bounds.SetMinMoves(getInt("min-moves"))
	}
	if flags.Changed("max-moves") {
		o.Bounds.SetMaxMoves(getInt("max-moves"))
	}
	if flags.Changed("moves") {
		o.Bounds.SetMoves(getInt("moves"))
	}
	if flags.Changed("min-ply") {
		o.Bounds.SetMinPly(getInt("min-ply"))
	}
	if flags.Changed("max-ply") {
		o.Bounds.SetMaxPly(getInt("max-ply"))
	}
	if flags.Changed("ply") {
		o.Bounds.SetPly(getInt("ply"))
	}
	if o.Bounds.MaxPly != 0 && o.Bounds.MinPly > o.Bounds.MaxPly {
		return nil, sel, errors.New("lower length bound exceeds the upper bound")
	}

	o.Checkmate = getBool("checkmate")
	o.Stalemate = getBool("stalemate")
	o.Insufficient = getBool("insufficient")
	if o.Checkmate && (o.Stalemate || o.Insufficient) {
		return nil, sel, errors.New("--checkmate cannot be combined with --stalemate or --insufficient")
	}
	o.Underpromotion = getBool("underpromotion")
	o.Commented = getBool("commented")
	switch {
	case getBool("fifty") && getBool("seventy-five"):
		return nil, sel, errors.New("--fifty and --seventy-five are mutually exclusive")
	case getBool("fifty"):
		o.NMoveRule = 50
	case getBool("seventy-five"):
		o.NMoveRule = 75
	}
	if n := getInt("repetition"); n < 0 {
		return nil, sel, errors.New("--repetition must be positive")
	} else {
		o.Repetition = n
	}
	if n := getInt("piece-count"); n < 0 {
		return nil, sel, errors.New("--piece-count must be positive")
	} else {
		o.PieceCount = n
	}
	if o.HigherRatedWinner && o.LowerRatedWinner {
		return nil, sel, errors.New("--higher-rated-winner and --lower-rated-winner are mutually exclusive")
	}

	for _, spec := range []struct {
		flag, fileFlag string
		colours        filter.Colours
	}{
		{"material", "material-file", filter.EitherColour},
		{"white-material", "white-material-file", filter.WhiteFirst},
	} {
		for _, path := range getList(spec.fileFlag) {
			f, err := os.Open(path)
			if err != nil {
				return nil, sel, err
			}
			ps, err := filter.ReadMaterialFile(f, spec.colours)
			f.Close()
			if err != nil {
				return nil, sel, fmt.Errorf("%s: %w", path, err)
			}
			o.Material = append(o.Material, ps...)
		}
		for _, line := range getList(spec.flag) {
			p, err := filter.ParseMaterial(line, spec.colours)
			if err != nil {
				return nil, sel, fmt.Errorf("--%s: %w", spec.flag, err)
			}
			o.Material = append(o.Material, p)
		}
	}
	o.AddMatchTag = getBool("add-match-tag")
	o.MarkMatches = getStr("mark-matches")

	o.NoBadResults = getBool("no-bad-results")
	o.FixResultTags = getBool("fix-result-tags")

	var err error
	if s := getStr("select-only"); s != "" {
		if sel.SelectOnly, err = filter.ParseRanges(s); err != nil {
			return nil, sel, fmt.Errorf("--select-only: %w", err)
		}
	}
	if s := getStr("skip-matching"); s != "" {
		if sel.SkipMatching, err = filter.ParseRanges(s); err != nil {
			return nil, sel, fmt.Errorf("--skip-matching: %w", err)
		}
	}
	for _, name := range []string{"stop-after", "first-game", "game-limit"} {
		if getInt(name) < 0 {
			return nil, sel, fmt.Errorf("--%s must be positive", name)
		}
	}
	sel.StopAfter = getInt("stop-after")
	sel.FirstGame = getInt("first-game")
	sel.GameLimit = getInt("game-limit")
	if sel.FirstGame > 0 && sel.GameLimit > 0 && sel.FirstGame > sel.GameLimit {
		return nil, sel, errors.New("--first-game is beyond --game-limit")
	}
	return o, sel, nil
}

func runFilter(cmd *cobra.Command, args []string) error {
	opts, err := outputOptions(cmd)
	if err != nil {
		return err
	}
	fopts, selOpts, err := filterOptions(cmd)
	if err != nil {
		return err
	}
	flags := cmd.Flags()
	keepBroken, _ := flags.GetBool("keep-broken")
	quiet, _ := flags.GetBool("quiet")
	suppressMatched, _ := flags.GetBool("suppress-matched")
	unmatchedPath, _ := flags.GetString("unmatched")

	out, outCloser, err := openOutput(cmd, "output")
	if err != nil {
		return err
	}
	if outCloser != nil {
		defer outCloser.Close()
	}
	var unmatchedOut io.Writer
	if unmatchedPath != "" {
		var closer io.Closer
		unmatchedOut, closer, err = openOutput(cmd, "unmatched")
		if err != nil {
			return err
		}
		if closer != nil {
			defer closer.Close()
		}
	}

	logW, logCloser, err := logWriter(cmd)
	if err != nil {
		return err
	}
	if logCloser != nil {
		defer logCloser.Close()
	}

	sourceOpts := output.Options{Format: output.Source, LineWidth: opts.LineWidth}
	matchedW := output.NewWriter(out, opts)
	matchedSourceW := output.NewWriter(out, sourceOpts)
	var unmatchedW, unmatchedSourceW *output.Writer
	if unmatchedOut != nil {
		unmatchedW = output.NewWriter(unmatchedOut, opts)
		unmatchedSourceW = output.NewWriter(unmatchedOut, sourceOpts)
	}

	sel := filter.NewSelector(selOpts)
	total, succeeded, ioErr := forEachGame(cmd, args, logW, logW, func(src string, n int, g *pgn.Game) error {
		r, replayErr := replay.Game(g, replay.Options{})
		if replayErr != nil && !keepBroken {
			return replayErr
		}
		matched, err := filter.Evaluate(g, r, fopts)
		if err != nil {
			return err
		}
		d := sel.Decide(n, matched)
		switch {
		case d.Emit && !suppressMatched:
			err = writeGame(g, r, replayErr, matchedW, matchedSourceW)
		case d.ToUnmatched && unmatchedW != nil:
			err = writeGame(g, r, replayErr, unmatchedW, unmatchedSourceW)
		}
		if err != nil {
			return err
		}
		if d.Stop {
			return stopAfter(replayErr)
		}
		return replayErr
	})

	if !quiet && !suppressMatched {
		matched := sel.Matched()
		fmt.Fprintf(logW, "%d game%s matched out of %d.\n", matched, plural(matched), total)
	}

	if ioErr != nil || succeeded < total {
		return errProblems
	}
	return nil
}

// writeGame writes g with w, or (when its replay failed and the game is
// being kept) with sourceW, since only Source format can render a game
// without replay detail.
func writeGame(g *pgn.Game, r *replay.Result, replayErr error, w, sourceW *output.Writer) error {
	if replayErr != nil {
		return sourceW.WriteGame(g, nil)
	}
	return w.WriteGame(g, r)
}
