package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"

	"github.com/kyleboon/pgnx/filter"
	"github.com/kyleboon/pgnx/output"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/pipeline"
	"github.com/spf13/cobra"
)

// routeTarget names the destination a route sends a processed game to.
type routeTarget int

const (
	// nowhere: the game is not written (validate; a filter game the
	// Selector held back; a matched game under --suppress-matched).
	nowhere routeTarget = iota
	// toOutput: the run's main destination (--output, or the --eco-split
	// files).
	toOutput
	// toUnmatched: the run's unmatched destination (filter --unmatched);
	// nowhere when the run has none.
	toUnmatched
)

// route is a command's routing decision for one processed game that is
// not Rejected (a Rejected game goes nowhere, whatever the run was told,
// so routes never see one): where it is written, and whether the run
// stops after it. stop is the Selector's decision; it is returned
// alongside the pipeline's Outcome, never folded into it. n is the game's
// 1-based number across all inputs.
type route func(n int, o pipeline.Outcome) (target routeTarget, stop bool)

// routeAll is convert's route: every game to the output, never stopping.
func routeAll(int, pipeline.Outcome) (routeTarget, bool) { return toOutput, false }

// routeNowhere is validate's route: nothing is written; the game is only
// reported.
func routeNowhere(int, pipeline.Outcome) (routeTarget, bool) { return nowhere, false }

// run is one invocation of a command that sends games through the
// pipeline: the setup convert and filter share (newRun) and the per-game
// sequence Observe -> Process -> route -> write (game). The commands
// differ only in their route, their unmatched destination and how the
// summary line counts.
type run struct {
	cmd  *cobra.Command
	opts output.Options
	// tags orders unrecognised tags run-wide in first-seen order; every
	// parsed game is observed before anything is written.
	tags *output.TagRegistry
	pipe *pipeline.Pipeline
	// logW takes per-game diagnostics, I/O errors and the summary line.
	logW io.Writer
	// quiet suppresses the summary line.
	quiet bool
	// route decides where each processed game goes.
	route route
	// output is the main destination; unmatched is the --unmatched
	// destination, nil when the command has none.
	output, unmatched destination
	// stdout is the one buffer every destination on stdout shares, so
	// that games to --output and --unmatched, both "-", stay in input
	// order; created on first use.
	stdout *bufio.Writer
	// matched counts the games the summary line reports as matched; nil
	// means the games that succeeded (convert). filter reports the
	// Selector's count instead.
	matched func() int
	// closers are the destinations, closed by closeDestinations last
	// opened first; logCloser is the --log file, closed after them by
	// close.
	closers   []io.Closer
	logCloser io.Closer
}

// newRun does the setup shared by convert and filter from the flags
// addOutputFlags and addECOFlags register: output options, ECO options,
// --keep-broken, --quiet, the log writer, the tag registry (seeded with
// the tag names the criteria mention, as the C lexer registers them from
// -t files first), the main destination and the pipeline. criteria is
// nil for convert. Every error is a usage error (exit 1). The caller
// must close the run.
func newRun(cmd *cobra.Command, criteria *filter.Options) (*run, error) {
	opts, err := outputOptions(cmd)
	if err != nil {
		return nil, err
	}
	ecoSet, err := ecoOptions(cmd)
	if err != nil {
		return nil, err
	}
	flags := cmd.Flags()
	keepBroken, _ := flags.GetBool("keep-broken")
	quiet, _ := flags.GetBool("quiet")

	r := &run{cmd: cmd, quiet: quiet, route: routeAll}
	r.logW, r.logCloser, err = logWriter(cmd)
	if err != nil {
		return nil, err
	}
	if !quiet {
		ecoPath, _ := flags.GetString("eco-file")
		ecoSet.logSkipped(r.logW, ecoPath)
	}

	// One registry shared by every writer, fed every parsed game (matched
	// or not) so that unrecognised tags are ordered as the C oracle
	// orders them: run-wide first-seen, criteria tag names first.
	opts.Tags = output.NewTagRegistry()
	for _, name := range criteria.TagNames() {
		opts.Tags.ObserveName(name)
	}
	if opts.SevenTagRoster && ecoSet.classifying() {
		opts.RosterExtras = ecoRosterExtras
	}
	r.opts, r.tags = opts, opts.Tags

	r.output, err = r.openDestination("output", ecoSet.split)
	if err != nil {
		r.close()
		return nil, err
	}
	r.closers = append(r.closers, r.output)

	r.pipe = pipeline.New(pipeline.Config{ECO: ecoSet.table, KeepBroken: keepBroken, Criteria: criteria})
	return r, nil
}

// newReportOnlyRun is the run of a command that writes no games
// (validate): the pipeline has no ECO table, no criteria and does not
// keep broken games, so a game fails exactly where convert would fail
// on it, and every game is routed nowhere. It has no destinations, no
// output options and no log of its own, so only game may be called on
// it (exec and close need the destinations and the log that newRun
// opens); the caller drives it through a runReport of its own. The tag
// registry is observed, as game requires, and never read.
func newReportOnlyRun() *run {
	return &run{
		tags:  output.NewTagRegistry(),
		pipe:  pipeline.New(pipeline.Config{}),
		route: routeNowhere,
	}
}

// openUnmatched opens the file named by the string flag flagName as the
// run's unmatched destination. Unmatched games are never divided by ECO
// (grammar.c select_output_file routes only matched games through -E).
func (r *run) openUnmatched(flagName string) error {
	w, err := r.openDestination(flagName, 0)
	if err != nil {
		return err
	}
	r.unmatched = w
	r.closers = append(r.closers, w)
	return nil
}

// closeDestinations flushes and closes the destinations, last opened
// first, and returns their close errors joined. A destination that
// could not be written in full (a full disk shows up at the final
// flush) surfaces here even though every game was handed to its writer
// without error; exec reports it through the run report, so the run
// exits 2. A second call is a no-op.
func (r *run) closeDestinations() error {
	var errs []error
	for i := len(r.closers) - 1; i >= 0; i-- {
		if err := r.closers[i].Close(); err != nil {
			errs = append(errs, err)
		}
	}
	r.closers = nil
	return errors.Join(errs...)
}

// close closes the destinations (if exec has not already), then the
// log file. A destination close error is reported on the log writer
// and makes close return errProblems.
func (r *run) close() error {
	var err error
	if cerr := r.closeDestinations(); cerr != nil {
		fmt.Fprintf(r.logW, "%v\n", cerr)
		err = errProblems
	}
	if r.logCloser != nil {
		r.logCloser.Close()
		r.logCloser = nil
	}
	return err
}

// finish closes the run once exec has returned err, exec's verdict, and
// returns that verdict, or close's when exec had none.
func (r *run) finish(err error) error {
	if cerr := r.close(); err == nil {
		err = cerr
	}
	return err
}

// game is the per-game sequence: observe the game's tags (before anything
// is written, so unrecognised tags order as the oracle's do), run the
// pipeline, route the outcome, write. The verdict carries the outcome's
// faults (the reason a Rejected game is emitted nowhere; a Broken game's
// replay failure and any warnings of a game that was emitted) or the
// failure of writing it, and the route's stop.
func (r *run) game(n int, g *pgn.Game) gameVerdict {
	r.tags.Observe(g)
	o := r.pipe.Process(g)
	if o.Status == pipeline.Rejected {
		return gameVerdict{Errors: o.Faults()}
	}
	target, stop := r.route(n, o)
	if err := r.write(target, o); err != nil {
		return gameVerdict{Errors: []error{err}, Stop: stop}
	}
	return gameVerdict{Errors: o.Faults(), Stop: stop}
}

// write sends o's game to the destination target names, with the replay
// the outcome says to emit: none for a Broken game, which the writer
// then renders in Source format, the only format a broken game is
// emitted in (CONFORMANCE.md, --keep-broken).
func (r *run) write(target routeTarget, o pipeline.Outcome) error {
	var d destination
	switch target {
	case toOutput:
		d = r.output
	case toUnmatched:
		d = r.unmatched
	}
	if d == nil {
		return nil
	}
	return writeGameTo(d, o.Game, o.ReplayToEmit())
}

// exec reads every game from args (or stdin) through game, closes the
// destinations, prints the summary line and returns the report's exit
// decision: errProblems (exit 2) when any game failed, an input could
// not be read or a destination could not be closed (a buffered
// destination flushes at close, so a failed flush surfaces there).
func (r *run) exec(args []string) error {
	rep := newRunReport(r.logW, r.logW)
	rep.quiet = r.quiet
	rep.each(games(args, r.cmd.InOrStdin(), parserOptions(r.cmd)), r.game)
	if err := r.closeDestinations(); err != nil {
		rep.fail(err)
	}
	count := rep.succeeded()
	if r.matched != nil {
		count = r.matched()
	}
	rep.summary(r.logW, count)
	return rep.exit()
}

// openDestination opens the destination for games: with --eco-split a
// per-ECO-prefix set of files in the current directory, otherwise the
// file named by the string flag flagName, or stdout (through the run's
// shared stdout buffer). The returned Closer flushes and closes whatever
// was opened.
func (r *run) openDestination(flagName string, splitLevel int) (destination, error) {
	if splitLevel > 0 {
		return newECOSplitDestination(splitLevel, r.opts), nil
	}
	out, closer, err := openOutput(r.cmd, flagName)
	if err != nil {
		return nil, err
	}
	if closer == nil {
		// stdout: one buffer for every destination on it.
		if r.stdout == nil {
			r.stdout = bufio.NewWriter(out)
		}
		return &bufferedDestination{w: output.NewWriter(r.stdout, r.opts), buf: r.stdout}, nil
	}
	return newBufferedDestination(out, closer, r.opts), nil
}

// plural returns "" when n == 1, else "s", matching the C oracle's
// "%lu game%s matched out of %lu.\n" wording (main.c).
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
