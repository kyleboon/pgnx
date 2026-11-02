package main

import (
	"errors"
	"io"

	"github.com/kyleboon/pgnx/filter"
	"github.com/kyleboon/pgnx/output"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/pipeline"
	"github.com/spf13/cobra"
)

// destination is where a route sends a processed game.
type destination int

const (
	// nowhere: the game is not written (validate; a filter game the
	// Selector held back; a matched game under --suppress-matched).
	nowhere destination = iota
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
type route func(n int, o pipeline.Outcome) (dest destination, stop bool)

// routeAll is convert's route: every game to the output, never stopping.
func routeAll(int, pipeline.Outcome) (destination, bool) { return toOutput, false }

// routeNowhere is validate's route: nothing is written; the game is only
// reported.
func routeNowhere(int, pipeline.Outcome) (destination, bool) { return nowhere, false }

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
	output, unmatched gameWriters
	// matched counts the games the summary line reports as matched; nil
	// means the games that succeeded (convert). filter reports the
	// Selector's count instead.
	matched func() int
	// closers are the destinations, closed by closeDestinations last
	// opened first; logCloser is the --log file, closed last by close.
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

	r.output, err = openDestination(cmd, "output", opts, ecoSet.split)
	if err != nil {
		r.close()
		return nil, err
	}
	r.closers = append(r.closers, r.output)

	r.pipe = pipeline.New(pipeline.Config{ECO: ecoSet.table, KeepBroken: keepBroken, Criteria: criteria})
	return r, nil
}

// openUnmatched opens the file named by the string flag flagName as the
// run's unmatched destination. Unmatched games are never divided by ECO
// (grammar.c select_output_file routes only matched games through -E).
func (r *run) openUnmatched(flagName string) error {
	w, err := openDestination(r.cmd, flagName, r.opts, 0)
	if err != nil {
		return err
	}
	r.unmatched = w
	r.closers = append(r.closers, w)
	return nil
}

// closeDestinations closes the destinations, last opened first, and
// returns their close errors joined; exec reports them before deciding
// the exit status. A second call is a no-op.
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

// close closes the destinations (if exec has not already) and the log
// file.
func (r *run) close() {
	r.closeDestinations()
	if r.logCloser != nil {
		r.logCloser.Close()
		r.logCloser = nil
	}
}

// game is the per-game sequence: observe the game's tags (before anything
// is written, so unrecognised tags order as the oracle's do), run the
// pipeline, route the outcome, write. The verdict carries the reason a
// Rejected game is emitted nowhere, else the problems of a game that was
// emitted (or the failure of writing it), and the route's stop.
func (r *run) game(n int, g *pgn.Game) gameVerdict {
	r.tags.Observe(g)
	o := r.pipe.Process(g)
	if o.Status == pipeline.Rejected {
		return gameVerdict{Errors: []error{o.Reason}}
	}
	dest, stop := r.route(n, o)
	if err := r.write(dest, o); err != nil {
		return gameVerdict{Errors: []error{err}, Stop: stop}
	}
	return verdict(o, stop)
}

// write sends o's game to dest, in Source format when the game is Broken.
func (r *run) write(dest destination, o pipeline.Outcome) error {
	var w gameWriters
	switch dest {
	case toOutput:
		w = r.output
	case toUnmatched:
		w = r.unmatched
	}
	if w == nil {
		return nil
	}
	return writeGameTo(w, o.Game, o.Replay, o.Status == pipeline.Broken)
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
