package main

import (
	"errors"
	"fmt"

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
	fuzzyDepth, _ := flags.GetInt("fuzzy-depth")
	sameSetup, _ := flags.GetBool("same-setup")
	if flags.Changed("fuzzy-depth") && fuzzyDepth < 0 {
		return errors.New("--fuzzy-depth cannot be negative")
	}
	if outPath, _ := flags.GetString("output"); dupPath != "" && dupPath == outPath {
		return fmt.Errorf("--duplicates %s is the --output file", dupPath)
	}
	if unmatchedPath, _ := flags.GetString("unmatched"); dupPath != "" && dupPath == unmatchedPath {
		return fmt.Errorf("--duplicates %s is the --unmatched file", dupPath)
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
		if err := r.openDuplicates("duplicates"); err != nil {
			r.close()
			return err
		}
	}

	log := dedup.NewLog(dedup.Options{Fuzzy: flags.Changed("fuzzy-depth"), FuzzyDepth: fuzzyDepth})
	divert := r.duplicates != nil

	// The visit (hashing.c previous_occurance) happens once all criteria
	// have passed and before the Selector's bookkeeping, which is the C
	// order; the disposition then says whether the game counts as
	// matched (grammar.c deal_with_game: a dropped duplicate, or a first
	// occurrence under -U, leaves game_matches false).
	var (
		duplicate bool
		origin    string
	)
	bySelector := routeBySelector(sel, suppressMatched, func(it Item, o pipeline.Outcome) bool {
		duplicate, origin = false, ""
		if !o.Matched {
			return false
		}
		origin, duplicate = log.Visit(o.Replay, it.Source)
		switch {
		case duplicate && !divert && !onlyDuplicates:
			return false // -D: dropped
		case !duplicate && onlyDuplicates:
			return false // -U: only duplicates are kept
		}
		return true
	})
	// lastFrom is the input the last diverted duplicate came from
	// (grammar.c's last_input_file): the From line is written when it
	// changes.
	lastFrom := ""
	r.route = func(it Item, o pipeline.Outcome) decision {
		d := bySelector(it, o)
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

// originName is the name the First found in line gives a source: the
// input file, or "_stdin_" for standard input (hashing.c
// previous_occurance: with no file name there is nothing to report).
func originName(source string) string {
	if source == "stdin" {
		return "_stdin_"
	}
	return source
}
