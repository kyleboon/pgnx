package main

import (
	"fmt"
	"io"
	"iter"

	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/pipeline"
)

// gameVerdict is a command's verdict on one game, distinct from the
// pipeline's Outcome: what the report should log and count, and
// whether the run ends after this game.
type gameVerdict struct {
	// Errors are why the game failed: the reason a Rejected game was
	// emitted nowhere, a Broken game's replay failure, the warnings of a
	// game emitted anyway, a failed write. Empty when the game
	// succeeded.
	Errors []error
	// Stop ends the run after this game (the Selector's --stop-after). It
	// is a routing fact, not a failure: the game's Errors are still
	// logged and counted.
	Stop bool
}

// runReport consumes the per-game verdicts of a run and owns the
// counting rules, the summary lines and the exit decision. Every
// command uses one.
type runReport struct {
	// w takes per-game diagnostics ("<src>:<line>: game <n>: <err>");
	// ioW takes the failures of sources and of the run itself
	// ("<src>: <err>"). They are separate so validate --summary can
	// discard findings without hiding I/O errors; the other commands
	// pass the log writer twice.
	w, ioW io.Writer
	// quiet suppresses the summary line.
	quiet bool
	// ioOnly makes exit disregard failed games: info reports statistics
	// over every game, so only an unreadable input fails its run.
	ioOnly bool

	// total is the number of games seen; failed those with any error or
	// any skipped token.
	total, failed int
	// ioFailed records that a source could not be read or the run itself
	// failed.
	ioFailed bool
}

func newRunReport(w, ioW io.Writer) *runReport {
	return &runReport{w: w, ioW: ioW}
}

// game records one item of the games iterator with the command's
// verdict on it. An Err item is a source's failure: logged to ioW, not
// a game. A game's skipped tokens are logged as warnings, then its
// errors; it counts as failed if it has either.
func (rep *runReport) game(it Item, v gameVerdict) {
	if it.Err != nil {
		rep.fail(fmt.Errorf("%s: %w", it.Source, it.Err))
		return
	}
	rep.total++
	for _, err := range it.Skipped {
		fmt.Fprintf(rep.w, "%s:%d: game %d: warning: %v\n", it.Source, it.Game.StartLine, it.Number, err)
	}
	for _, err := range v.Errors {
		fmt.Fprintf(rep.w, "%s:%d: game %d: %v\n", it.Source, it.Game.StartLine, it.Number, err)
	}
	if len(it.Skipped) > 0 || len(v.Errors) > 0 {
		rep.failed++
	}
}

// each drives fn over every game of seq (the games iterator), recording
// each verdict, and stops reading as soon as a verdict says so. Err
// items go straight to the report.
func (rep *runReport) each(seq iter.Seq[Item], fn func(n int, g *pgn.Game) gameVerdict) {
	for it := range seq {
		var v gameVerdict
		if it.Err == nil {
			v = fn(it.Number, it.Game)
		}
		rep.game(it, v)
		if v.Stop {
			return
		}
	}
}

// verdict folds a pipeline outcome into the verdict on a game that was
// routed (so not Rejected): the reason it is Broken and any warnings
// are its errors, and stop is the route's decision.
func verdict(o pipeline.Outcome, stop bool) gameVerdict {
	v := gameVerdict{Stop: stop}
	if o.Reason != nil {
		v.Errors = append(v.Errors, o.Reason)
	}
	v.Errors = append(v.Errors, o.Warnings...)
	return v
}

// fail records a failure of the run that is not a game's: an input that
// could not be read, a destination that could not be flushed at close.
// It is logged to ioW and fails the run.
func (rep *runReport) fail(err error) {
	fmt.Fprintf(rep.ioW, "%v\n", err)
	rep.ioFailed = true
}

// succeeded is the number of games with no error and no skipped token.
func (rep *runReport) succeeded() int {
	return rep.total - rep.failed
}

// summary writes the pgn-extract summary line (main.c: "%lu game%s
// matched out of %lu.") with the count the command reports as matched,
// unless quiet.
func (rep *runReport) summary(w io.Writer, matched int) {
	if rep.quiet {
		return
	}
	fmt.Fprintf(w, "%d game%s matched out of %d.\n", matched, plural(matched), rep.total)
}

// validSummary writes validate's tally line.
func (rep *runReport) validSummary(w io.Writer) {
	fmt.Fprintf(w, "%d of %d games valid\n", rep.succeeded(), rep.total)
}

// exit decides the exit status: errProblems (exit 2) when a source or
// the run failed, or, unless ioOnly, when any game failed; nil (exit 0)
// otherwise.
func (rep *runReport) exit() error {
	if rep.ioFailed || (!rep.ioOnly && rep.failed > 0) {
		return errProblems
	}
	return nil
}
