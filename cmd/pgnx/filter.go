package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/kyleboon/pgnx/filter"
	"github.com/kyleboon/pgnx/pipeline"
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
	addECOFlags(cmd)

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

	flags.StringArray("position", nil, "match games reaching the position after these SAN moves from the start, by any move order (-x line; repeatable)")
	flags.StringArray("position-file", nil, "file of --position move lines, one per line (-x FILE; repeatable)")
	flags.StringArray("fen", nil, "match games reaching the piece placement of this FEN (FEN criteria line; repeatable)")
	flags.StringArray("fen-pattern", nil, "match games reaching a position matching this FEN pattern (-Tf / --fenpattern; repeatable)")
	flags.StringArray("fen-pattern-i", nil, "as --fen-pattern, also matching the colour-reversed pattern (--fenpatterni; repeatable)")
	flags.StringArray("hash", nil, "match games reaching the position with this Polyglot hash, in hex (-H; repeatable)")
	flags.Int("start-ply", 1, "start positional, repetition and N-move-rule matching at ply N (--startply)")
	flags.Int("match-ply-limit", 0, "search for positions only up to ply N (--matchplylimit)")
	flags.Bool("white-to-move", false, "only count positions with White to move (--wtm)")
	flags.Bool("black-to-move", false, "only count positions with Black to move (--btm)")
	flags.Bool("add-label-tag", false, "add a MatchLabel tag holding the label of the FEN pattern that matched (--addlabeltag)")
	flags.StringArray("sequence", nil, "match games whose moves include these moves, in any order unless --exact-order; * matches any move, !move forbids one, a|b offers alternatives (-v line; repeatable)")
	flags.StringArray("sequence-file", nil, "file of --sequence lines, one per line (-v FILE; repeatable)")
	flags.Bool("exact-order", false, "require --sequence moves in the listed order (-P)")
	flags.Bool("sequence-anywhere", false, "let a --sequence match start at any move, not only the first (--vanywhere)")

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

// filterOptions maps the filter flags onto filter.Options and
// filter.SelectorOptions. It checks only what the flags themselves can
// get wrong (a value that cannot be mapped, two flags for one setting);
// the criteria's own invariants are filter's, checked by Validate. Every
// error it returns is a usage error (exit 1).
func filterOptions(cmd *cobra.Command) (*filter.Options, filter.SelectorOptions, error) {
	flags := cmd.Flags()
	getBool := func(name string) bool { v, _ := flags.GetBool(name); return v }
	getInt := func(name string) int { v, _ := flags.GetInt(name); return v }
	getStr := func(name string) string { v, _ := flags.GetString(name); return v }
	getList := func(name string) []string { v, _ := flags.GetStringArray(name); return v }

	o := &filter.Options{
		Tags:      &filter.TagMatcher{Substring: getBool("tag-substring"), Soundex: getBool("soundex")},
		Positions: &filter.PositionCriteria{},
	}
	var sel filter.SelectorOptions

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
			if err := o.AddCriterion(c); err != nil {
				return nil, sel, fmt.Errorf("%s: %w", path, err)
			}
		}
	}
	for _, expr := range getList("tag") {
		c, err := filter.ParseCriterion(expr)
		if err != nil {
			return nil, sel, fmt.Errorf("--tag: %w", err)
		}
		if err := o.AddCriterion(c); err != nil {
			return nil, sel, fmt.Errorf("--tag: %w", err)
		}
	}
	for flag, tag := range map[string]string{
		"player": "Player", "white": "White", "black": "Black",
		"date": "Date", "result": "Result", "eco": "ECO",
	} {
		for _, v := range getList(flag) {
			o.Tags.AddPrefix(tag, v)
		}
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

	o.Checkmate = getBool("checkmate")
	o.Stalemate = getBool("stalemate")
	o.Insufficient = getBool("insufficient")
	o.Underpromotion = getBool("underpromotion")
	o.Commented = getBool("commented")
	for flag, rule := range map[string]int{"fifty": 50, "seventy-five": 75} {
		if getBool(flag) {
			if err := o.SetNMoveRule(rule); err != nil {
				return nil, sel, fmt.Errorf("--%s: %w", flag, err)
			}
		}
	}
	o.Repetition = getInt("repetition")
	o.PieceCount = getInt("piece-count")

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

	// Positional criteria (-x, FEN, -H, -Tf) and their modifiers.
	pc := o.Positions
	for _, path := range getList("position-file") {
		f, err := os.Open(path)
		if err != nil {
			return nil, sel, err
		}
		lines, err := filter.ReadPositionLines(f)
		f.Close()
		if err != nil {
			return nil, sel, fmt.Errorf("%s: %w", path, err)
		}
		for _, line := range lines {
			if err := pc.AddLine(line); err != nil {
				return nil, sel, fmt.Errorf("%s: position line %q: %w", path, line, err)
			}
		}
	}
	for _, line := range getList("position") {
		if err := pc.AddLine(line); err != nil {
			return nil, sel, fmt.Errorf("--position %q: %w", line, err)
		}
	}
	for _, fen := range getList("fen") {
		if err := pc.AddFEN(fen); err != nil {
			return nil, sel, fmt.Errorf("--fen: %w", err)
		}
	}
	for _, p := range getList("fen-pattern") {
		if err := pc.AddPattern(p, false, ""); err != nil {
			return nil, sel, fmt.Errorf("--fen-pattern: %w", err)
		}
	}
	for _, p := range getList("fen-pattern-i") {
		if err := pc.AddPattern(p, true, ""); err != nil {
			return nil, sel, fmt.Errorf("--fen-pattern-i: %w", err)
		}
	}
	for _, h := range getList("hash") {
		if err := pc.AddHash(h); err != nil {
			return nil, sel, fmt.Errorf("--hash %q: %w", h, err)
		}
	}
	switch {
	case getBool("white-to-move") && getBool("black-to-move"):
		return nil, sel, errors.New("--white-to-move and --black-to-move are mutually exclusive")
	case getBool("white-to-move"):
		pc.WhoseMove = filter.WhiteToMove
	case getBool("black-to-move"):
		pc.WhoseMove = filter.BlackToMove
	}
	if flags.Changed("match-ply-limit") {
		if pc.MatchPlyLimit = getInt("match-ply-limit"); pc.MatchPlyLimit < 1 {
			return nil, sel, errors.New("--match-ply-limit must be at least 1")
		}
	}
	if n := getInt("start-ply"); n < 1 {
		return nil, sel, errors.New("--start-ply must be at least 1")
	} else {
		o.StartPly = n
	}
	o.AddLabelTag = getBool("add-label-tag")
	o.NoVariations = getBool("no-variations")

	// Move sequences (-v, -P, --vanywhere).
	for _, path := range getList("sequence-file") {
		f, err := os.Open(path)
		if err != nil {
			return nil, sel, err
		}
		seqs, err := filter.ReadSequences(f)
		f.Close()
		if err != nil {
			return nil, sel, fmt.Errorf("%s: %w", path, err)
		}
		o.Sequences = append(o.Sequences, seqs...)
	}
	for _, line := range getList("sequence") {
		seq, err := filter.ParseSequence(line)
		if err != nil {
			return nil, sel, fmt.Errorf("--sequence %q: %w", line, err)
		}
		if seq == nil {
			return nil, sel, fmt.Errorf("--sequence %q: no moves", line)
		}
		o.Sequences = append(o.Sequences, seq)
	}
	o.ExactOrder = getBool("exact-order")
	o.SequenceAnywhere = getBool("sequence-anywhere")

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
	sel.StopAfter = getInt("stop-after")
	sel.FirstGame = getInt("first-game")
	sel.GameLimit = getInt("game-limit")

	if err := o.Validate(); err != nil {
		return nil, sel, err
	}
	if err := sel.Validate(); err != nil {
		return nil, sel, err
	}
	return o, sel, nil
}

func runFilter(cmd *cobra.Command, args []string) error {
	fopts, selOpts, err := filterOptions(cmd)
	if err != nil {
		return err
	}
	r, err := newRun(cmd, fopts)
	if err != nil {
		return err
	}

	flags := cmd.Flags()
	suppressMatched, _ := flags.GetBool("suppress-matched")
	if unmatchedPath, _ := flags.GetString("unmatched"); unmatchedPath != "" {
		if err := r.openUnmatched("unmatched"); err != nil {
			r.close()
			return err
		}
	}

	// The Selector routes each processed game: matched games to the
	// output unless --suppress-matched, the rest to --unmatched, and its
	// stop-after decision ends the run.
	sel := filter.NewSelector(selOpts)
	r.route = func(n int, o pipeline.Outcome) (routeTarget, bool) {
		d := sel.Decide(n, o.Matched)
		switch {
		case d.Emit && !suppressMatched:
			return toOutput, d.Stop
		case d.ToUnmatched:
			return toUnmatched, d.Stop
		}
		return nowhere, d.Stop
	}
	// The summary counts the Selector's matches, and --suppress-matched
	// suppresses it along with the matched games.
	r.matched = sel.Matched
	r.quiet = r.quiet || suppressMatched
	return r.finish(r.exec(args))
}
