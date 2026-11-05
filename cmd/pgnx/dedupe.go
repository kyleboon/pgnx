package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/kyleboon/pgnx/dedup"
	"github.com/kyleboon/pgnx/pipeline"
	"github.com/spf13/cobra"
)

// newDedupeCmd builds the "pgnx dedupe" subcommand: filter's complete
// flag set (output, ECO, criteria, Selector, --unmatched) plus duplicate
// detection. In pgn-extract the duplicate check (-D, -d, -U, -c) sits
// after every criterion in deal_with_game and composes with all of them;
// dedupe with no dedup flags is filter plus "drop duplicates".
func newDedupeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dedupe [file...]",
		Short: "Drop, divert or keep only duplicate games",
		Long: `Detect duplicate games: a game whose moves reach the same final position
through the same sequence of positions as a game seen earlier in the run
(or in a --check-file). dedupe takes every filter flag, so criteria and the
Selector apply first and only games that match them are compared.

By default duplicates are dropped (pgn-extract -D): they are unmatched, so
--unmatched receives them and the summary does not count them. With
--duplicates FILE (-d) they are written to FILE instead, preceded by
"{ From: <input> }" and "{ First found in: <origin> }" comments unless
--no-comments; with --only-duplicates (-U) the first occurrences are
dropped and only the duplicates are written.

--check-file FILE (-c, repeatable) names games to treat as already seen:
they are read before every input, must satisfy the criteria to count, and
are never written. --fuzzy-depth N (--fuzzydepth) makes two games
duplicates when they reach the same position after ply N by any moves
(N = 0: the same final position). --same-setup (--deletesamesetup) drops
a game whose starting position (FEN tag, or the standard start) has been
seen before, before its moves are compared.`,
		RunE: runDedupe,
	}
	addOutputFlags(cmd)
	addECOFlags(cmd)
	addCriteriaFlags(cmd)

	flags := cmd.Flags()
	flags.String("duplicates", "", "write duplicate games to FILE instead of dropping them (-d; - for stdout)")
	flags.Bool("only-duplicates", false, "write only duplicate games; first occurrences are dropped (-U)")
	flags.StringArray("check-file", nil, "PGN file of games treated as already seen; read before the inputs, never written (-c; repeatable)")
	flags.Int("fuzzy-depth", 0, "duplicates share the position after ply N by any moves; 0 compares final positions (--fuzzydepth)")
	flags.Bool("same-setup", false, "drop a game whose starting position has been seen before (--deletesamesetup)")
	return cmd
}

func runDedupe(cmd *cobra.Command, args []string) error {
	fopts, selOpts, err := criteriaOptions(cmd)
	if err != nil {
		return err
	}
	flags := cmd.Flags()
	dupPath, _ := flags.GetString("duplicates")
	onlyDuplicates, _ := flags.GetBool("only-duplicates")
	checkFiles, _ := flags.GetStringArray("check-file")
	fuzzyDepth, _ := flags.GetInt("fuzzy-depth")
	sameSetup, _ := flags.GetBool("same-setup")
	if flags.Changed("fuzzy-depth") && fuzzyDepth < 0 {
		return errors.New("--fuzzy-depth cannot be negative")
	}
	if dupPath != "" {
		// "" and "-" both name stdout for --output; "-" does for the
		// others.
		outPath, _ := flags.GetString("output")
		unmatchedPath, _ := flags.GetString("unmatched")
		switch {
		case dupPath == outPath, dupPath == "-" && outPath == "":
			return fmt.Errorf("--duplicates %s is the --output destination", dupPath)
		case dupPath == unmatchedPath:
			return fmt.Errorf("--duplicates %s is the --unmatched file", dupPath)
		}
	}
	// lex.c must_open_file: a check file that cannot be opened ends the
	// run before any game is read.
	for _, path := range checkFiles {
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("--check-file: %w", err)
		}
		f.Close()
	}
	if sameSetup {
		// The one stateful criterion: it runs inside the chain, at the
		// check_duplicate_setup point of Evaluate.
		fopts.SameSetup = &dedup.SetupSet{}
	}

	r, err := newRun(cmd, fopts)
	if err != nil {
		return err
	}
	sel, suppressMatched, err := r.openSelection(selOpts)
	if err != nil {
		r.close()
		return err
	}
	if dupPath != "" {
		if err := r.openSide("duplicates", &r.duplicates); err != nil {
			r.close()
			return err
		}
	}

	seen := dedup.NewLog(dedup.Options{Fuzzy: flags.Changed("fuzzy-depth"), FuzzyDepth: fuzzyDepth})
	divert := r.duplicates != nil

	// lastFrom is the input the last diverted duplicate came from
	// (grammar.c's last_input_file): the From line is written when it
	// changes.
	lastFrom := ""
	seedCheckFiles(r, checkFiles, seen)
	r.route = func(it Item, o pipeline.Outcome) decision {
		// The visit (hashing.c previous_occurance) happens once all
		// criteria have passed and before the Selector's bookkeeping,
		// which is the C order; the disposition then says whether the
		// game counts as matched (grammar.c deal_with_game: a dropped
		// duplicate, or a first occurrence under -U, leaves game_matches
		// false). The game as parsed is what is compared (SourceReplay):
		// the C tool hashes it inside apply_move_list, before
		// rewrite_game drops any plies.
		matched, duplicate, origin := o.Matched, false, ""
		if matched {
			origin, duplicate = seen.Visit(o.SourceReplay, it.Source)
			switch {
			case duplicate && !divert && !onlyDuplicates:
				matched = false // -D: dropped
			case !duplicate && onlyDuplicates:
				matched = false // -U: only duplicates are kept
			}
		}
		d := decideBySelector(sel, suppressMatched, it.Number, matched)
		if duplicate && divert && d.target == toOutput {
			d.target = toDuplicates
			if !r.opts.NoComments {
				if it.Source != lastFrom {
					d.comments = append(d.comments, "From: "+it.Source)
					lastFrom = it.Source
				}
				d.comments = append(d.comments, "First found in: "+originName(origin))
			}
		}
		return d
	}
	return r.finish(r.exec(args))
}

// seedCheckFiles reads every game of the --check-file files through the
// run before the first input (lex.c stores CHECKFILEs ahead of the
// NORMALFILEs whatever the command-line order), registering in seen those
// that satisfy the criteria, as deal_with_game does for a CHECKFILE:
// the Selector never sees them, nothing is written, and they are neither
// counted by the summary nor able to fail the run (a game that does not
// parse or replay is logged and passed over). Their tags are observed, as
// the C lexer registers tag names from check files first. The run's
// route is the caller's to set afterwards.
func seedCheckFiles(r *run, files []string, seen *dedup.Log) {
	if len(files) == 0 {
		return
	}
	r.route = func(it Item, o pipeline.Outcome) decision {
		if o.Matched {
			seen.Visit(o.SourceReplay, it.Source)
		}
		return decision{target: nowhere}
	}
	rep := newRunReport(r.logW, r.logW)
	rep.quiet = true
	rep.each(games(files, nil, parserOptions(r.cmd)), r.game)
}

// originName is the name the First found in line gives a source: the
// input file, or "_stdin_" for standard input (hashing.c
// previous_occurance: with no file name there is nothing to report).
func originName(source string) string {
	if source == stdinSource {
		return "_stdin_"
	}
	return source
}
