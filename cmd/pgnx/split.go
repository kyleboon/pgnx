package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/kyleboon/pgnx/output"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/spf13/cobra"
)

// newSplitCmd builds the "pgnx split" subcommand, pgn-extract's -#N[,M]:
// convert, with the output divided into files of N games each, named by
// number from M in the current directory.
func newSplitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "split [file...]",
		Short: "Convert PGN games into numbered files of N games each",
		Long: `Convert PGN games as convert does, writing them to files of N games each
in the current directory, named 1.pgn, 2.pgn, ... (or .epd/.fen by format),
starting the numbering at --start. Every convert output flag applies;
--output and --eco-split do not.`,
		RunE: runSplit,
	}
	addOutputFlags(cmd)
	addECOFlags(cmd)
	flags := cmd.Flags()
	flags.Int("games", 0, "games per file (-# N; required)")
	flags.Int("start", 1, "number of the first file (-# N,M)")
	return cmd
}

func runSplit(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()
	perFile, _ := flags.GetInt("games")
	if perFile < 1 {
		return errors.New("--games must be at least 1")
	}
	start, _ := flags.GetInt("start")
	if out, _ := flags.GetString("output"); out != "" {
		// argsfile.c: "-o conflicts with -#".
		return errors.New("--output cannot be combined with split")
	}
	if append_, _ := flags.GetBool("append"); append_ {
		return errors.New("--append cannot be combined with split")
	}
	if flags.Changed("eco-split") {
		return errors.New("--eco-split cannot be combined with split")
	}
	r, err := newRunTo(cmd, nil, func(r *run) (destination, error) {
		return &splitDestination{perFile: perFile, next: start, suffix: fileSuffix(r.opts.Format), opts: r.opts}, nil
	})
	if err != nil {
		return err
	}
	return r.finish(r.exec(args))
}

// fileSuffix is output.c output_file_suffix: the extension of the files
// -# writes for a format.
func fileSuffix(f output.Format) string {
	switch f {
	case output.EPD:
		return ".epd"
	case output.FEN:
		return ".fen"
	case output.CM:
		return ".cm"
	}
	return ".pgn"
}

// splitDestination is the destination of a split run: every perFile
// games a new file "<next><suffix>" is created in the current directory
// (grammar.c select_output_file), buffered for its lifetime and closed
// when the next one opens or at Close. A file that could not be written
// in full fails the run at Close, like any destination, without
// disturbing the games that follow it.
type splitDestination struct {
	perFile, next int
	suffix        string
	opts          output.Options
	// count is the number of games written so far; cur is the file the
	// last one went to; err is the first failure closing a finished file.
	count int
	cur   *bufferedDestination
	err   error
}

func (s *splitDestination) writerFor(*pgn.Game) (*output.Writer, error) {
	if s.count%s.perFile == 0 {
		s.closeCurrent()
		bw, err := openFileDestination(fmt.Sprintf("%d%s", s.next, s.suffix), os.O_TRUNC, s.opts)
		if err != nil {
			return nil, err
		}
		s.cur = bw
		s.next++
	}
	s.count++
	return s.cur.w, nil
}

// closeCurrent flushes and closes the current file, if any, keeping the
// first failure for Close to report.
func (s *splitDestination) closeCurrent() {
	if s.cur == nil {
		return
	}
	if err := s.cur.Close(); err != nil && s.err == nil {
		s.err = err
	}
	s.cur = nil
}

// Close closes the current file and reports the first failure of any
// file the run wrote.
func (s *splitDestination) Close() error {
	s.closeCurrent()
	return s.err
}
