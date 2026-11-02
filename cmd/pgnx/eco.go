package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/kyleboon/pgnx/eco"
	"github.com/kyleboon/pgnx/output"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
	"github.com/spf13/cobra"
)

// ecoRosterExtras are the tags the C tool appends after the seven-tag
// roster when ECO classification is in force (output.c:1531-1539).
var ecoRosterExtras = []string{"ECO", "Opening", "Variation", "SubVariation"}

// maxECOSplitLevel is the C tool's MAX_ECO_LEVEL: the longest ECO-code
// prefix -E can divide output by.
const maxECOSplitLevel = 10

// addECOFlags registers the ECO flags shared by convert and filter.
func addECOFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.String("eco-file", "", "classify games by the ECO lines in FILE, replacing their ECO, Opening, Variation and SubVariation tags (-e)")
	flags.Int("eco-split", 0, "write games to <prefix>.pgn in the current directory, appending, where prefix is the first N characters of the ECO code, or noeco.pgn (-E N, 1..10; conflicts with --output)")
}

// ecoSettings is the parsed form of the ECO flags.
type ecoSettings struct {
	// table is the loaded --eco-file, nil when classification is off.
	table *eco.Table
	// split is the --eco-split level, 0 when output is not divided.
	split int
}

// ecoOptions validates the ECO flags and loads the --eco-file table.
// Every error it returns is a usage error (exit 1). --eco-split is
// accepted without --eco-file, dividing on the games' existing ECO tags
// as the C tool does.
func ecoOptions(cmd *cobra.Command) (ecoSettings, error) {
	flags := cmd.Flags()
	var s ecoSettings
	if flags.Changed("eco-split") {
		n, _ := flags.GetInt("eco-split")
		if n < 1 || n > maxECOSplitLevel {
			return s, fmt.Errorf("--eco-split must be between 1 and %d", maxECOSplitLevel)
		}
		if out, _ := flags.GetString("output"); out != "" {
			return s, errors.New("--eco-split and --output are mutually exclusive")
		}
		s.split = n
	}
	if path, _ := flags.GetString("eco-file"); path != "" {
		f, err := os.Open(path)
		if err != nil {
			return s, err
		}
		defer f.Close()
		s.table, err = eco.Load(f)
		if err != nil {
			return s, fmt.Errorf("--eco-file %s: %w", path, err)
		}
	}
	return s, nil
}

// classifying reports whether games are being classified.
func (s ecoSettings) classifying() bool { return s.table != nil }

// logSkipped reports ECO lines that could not be replayed, as the C tool
// logs each failure while reading the file.
func (s ecoSettings) logSkipped(w io.Writer, path string) {
	if s.table != nil && s.table.Skipped > 0 {
		fmt.Fprintf(w, "%s: skipped %d ECO line%s that could not be replayed.\n", path, s.table.Skipped, plural(s.table.Skipped))
	}
}

// gameWriter is a destination for games: the output.Writer a game is
// written with (one fixed writer, or one per ECO file under --eco-split).
// Every writer renders into one bufio.Writer per sink that lives for the
// whole run (ADR 0003: the output package writes a line at a time, so an
// unbuffered file would cost a syscall per tag line). Close flushes the
// buffers and closes the files, returning the first error, so an output
// that could not be written in full (a full disk) still fails the run.
type gameWriter interface {
	writerFor(g *pgn.Game) (*output.Writer, error)
	io.Closer
}

// bufferedWriter is one sink (a file, or stdout) with its run-wide
// buffer and the one output.Writer that renders into it.
type bufferedWriter struct {
	w   *output.Writer
	buf *bufio.Writer
	// file is closed after the final flush; nil for stdout.
	file io.Closer
}

// newBufferedWriter buffers dst for the run and closes file, when not
// nil, at Close.
func newBufferedWriter(dst io.Writer, file io.Closer, opts output.Options) *bufferedWriter {
	buf := bufio.NewWriter(dst)
	return &bufferedWriter{w: output.NewWriter(buf, opts), buf: buf, file: file}
}

func (b *bufferedWriter) writerFor(*pgn.Game) (*output.Writer, error) { return b.w, nil }

// Close flushes what the run buffered and closes the file. A flush error
// is the write error the sink refused with; it wins over a close error.
func (b *bufferedWriter) Close() error {
	err := b.buf.Flush()
	if b.file != nil {
		if cerr := b.file.Close(); err == nil {
			err = cerr
		}
	}
	return err
}

// ecoSplitWriters implements --eco-split: each game goes to the file named
// by eco.FilePrefix of its ECO tag, in the current directory, opened in
// append mode on first use and kept open (and buffered) for the run
// (eco.c open_eco_output_file; the C tool reopens per game, which is
// observably the same). All writers share the run's options and
// TagRegistry.
type ecoSplitWriters struct {
	level int
	opts  output.Options
	files map[string]*bufferedWriter
}

func newECOSplitWriters(level int, opts output.Options) *ecoSplitWriters {
	return &ecoSplitWriters{level: level, opts: opts, files: map[string]*bufferedWriter{}}
}

func (s *ecoSplitWriters) writerFor(g *pgn.Game) (*output.Writer, error) {
	ecoTag, _ := g.Tag("ECO")
	name := eco.FilePrefix(ecoTag, s.level) + ".pgn"
	if bw, ok := s.files[name]; ok {
		return bw.w, nil
	}
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	bw := newBufferedWriter(f, f, s.opts)
	s.files[name] = bw
	return bw.w, nil
}

// Close flushes and closes every file opened so far, returning the first
// error.
func (s *ecoSplitWriters) Close() error {
	var first error
	for name, bw := range s.files {
		if err := bw.Close(); err != nil && first == nil {
			first = err
		}
		delete(s.files, name)
	}
	return first
}

// writeGameTo resolves the destination for g and writes it with its
// replay r: nil for a broken game, which the writer then renders in
// Source format, the only format that needs no replay.
func writeGameTo(dest gameWriter, g *pgn.Game, r *replay.Result) error {
	w, err := dest.writerFor(g)
	if err != nil {
		return err
	}
	return w.WriteGame(g, r)
}
