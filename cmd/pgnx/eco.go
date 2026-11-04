package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/kyleboon/pgnx/eco"
	"github.com/kyleboon/pgnx/output"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/spf13/cobra"
)

// ecoRosterExtras are the tags the C tool appends after the seven-tag
// roster when ECO classification is in force (output.c:1531-1539).
var ecoRosterExtras = []string{"ECO", "Opening", "Variation", "SubVariation"}

// maxECOSplitLevel is the C tool's MAX_ECO_LEVEL: the longest ECO-code
// prefix -E can divide output by.
const maxECOSplitLevel = 10

// addECOFlags registers the ECO flags shared by convert, filter and split.
func addECOFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.String("eco-file", "", "classify games by the ECO lines in FILE, replacing their ECO, Opening, Variation and SubVariation tags (-e)")
	flags.Bool("eco-default", false, "classify games by the ECO lines in $ECO_FILE, or eco.pgn in the current directory when it is unset or empty (-e with no file name; conflicts with --eco-file)")
	flags.Int("eco-split", 0, "write games to <prefix>.pgn in the current directory, appending, where prefix is the first N characters of the ECO code, or noeco.pgn (-E N, 1..10; conflicts with --output)")
}

// ecoSettings is the parsed form of the ECO flags.
type ecoSettings struct {
	// table is the loaded ECO file, nil when classification is off.
	table *eco.Table
	// path is the file table was loaded from: --eco-file, or the file
	// --eco-default resolved to.
	path string
	// split is the --eco-split level, 0 when output is not divided.
	split int
}

// ecoOptions validates the ECO flags and loads the ECO table named by
// --eco-file or resolved by --eco-default. Every error it returns is a
// usage error (exit 1). --eco-split is accepted without either,
// dividing on the games' existing ECO tags as the C tool does.
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
	path, _ := flags.GetString("eco-file")
	if useDefault, _ := flags.GetBool("eco-default"); useDefault {
		if path != "" {
			return s, errors.New("--eco-default and --eco-file are mutually exclusive")
		}
		path = defaultECOPath()
	}
	if path == "" {
		return s, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return s, err
	}
	defer f.Close()
	s.table, err = eco.Load(f)
	if err != nil {
		return s, fmt.Errorf("ECO file %s: %w", path, err)
	}
	s.path = path
	return s, nil
}

// defaultECOPath is the file -e reads with no file name: $ECO_FILE, or
// eco.pgn in the current directory (argsfile.c USE_ECO_FILE_ARGUMENT;
// main.c:51 DEFAULT_ECO_FILE). An empty $ECO_FILE counts as unset; the
// C tool would try to open "".
func defaultECOPath() string {
	if p := os.Getenv("ECO_FILE"); p != "" {
		return p
	}
	return "eco.pgn"
}

// classifying reports whether games are being classified.
func (s ecoSettings) classifying() bool { return s.table != nil }

// logSkipped reports ECO lines that could not be replayed, as the C tool
// logs each failure while reading the file.
func (s ecoSettings) logSkipped(w io.Writer) {
	if s.table != nil && s.table.Skipped > 0 {
		fmt.Fprintf(w, "%s: skipped %d ECO line%s that could not be replayed.\n", s.path, s.table.Skipped, plural(s.table.Skipped))
	}
}

// ecoSplitDestination is the destination of an --eco-split run: each
// game goes to the file named by eco.FilePrefix of its ECO tag, in the
// current directory, opened in append mode on first use and kept open
// (and buffered) for the run (eco.c open_eco_output_file; the C tool
// reopens per game, which is observably the same). All writers share
// the run's options and TagRegistry.
type ecoSplitDestination struct {
	level int
	opts  output.Options
	files map[string]*bufferedDestination
}

func newECOSplitDestination(level int, opts output.Options) *ecoSplitDestination {
	return &ecoSplitDestination{level: level, opts: opts, files: map[string]*bufferedDestination{}}
}

func (s *ecoSplitDestination) writerFor(g *pgn.Game) (*output.Writer, error) {
	ecoTag, _ := g.Tag("ECO")
	name := eco.FilePrefix(ecoTag, s.level) + ".pgn"
	if bw, ok := s.files[name]; ok {
		return bw.w, nil
	}
	bw, err := openFileDestination(name, os.O_APPEND, s.opts)
	if err != nil {
		return nil, err
	}
	s.files[name] = bw
	return bw.w, nil
}

// Close flushes and closes every file opened so far, returning the first
// error.
func (s *ecoSplitDestination) Close() error {
	var first error
	for name, bw := range s.files {
		if err := bw.Close(); err != nil && first == nil {
			first = err
		}
		delete(s.files, name)
	}
	return first
}
