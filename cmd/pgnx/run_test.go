package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyleboon/pgnx/output"
	"github.com/kyleboon/pgnx/pipeline"
	"github.com/spf13/cobra"
)

// TestFilterNoCriteriaEqualsConvertOverGoldenInputs pins "convert is
// filter with no criteria" over every golden input file, not just a
// spot check: for each file under testdata/golden/infiles (PGN or not;
// a file the scanner cannot read must fail both commands alike), convert
// and filter-with-no-criteria must produce byte-identical output,
// identical per-game diagnostics and the same exit code. --quiet keeps
// the summary line out of the comparison: convert counts the games that
// succeeded, filter the games the Selector matched, and the golden rows
// pin each. --keep-broken is run as well so the Broken route is covered.
func TestFilterNoCriteriaEqualsConvertOverGoldenInputs(t *testing.T) {
	dir := filepath.Join("testdata", "golden", "infiles")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatalf("no golden input files in %s", dir)
	}
	for _, variant := range []struct {
		name  string
		flags []string
	}{
		{"default", nil},
		{"keep-broken", []string{"--keep-broken"}},
	} {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			path := filepath.Join(dir, e.Name())
			t.Run(variant.name+"/"+e.Name(), func(t *testing.T) {
				run := func(sub string) (string, string, int) {
					args := append([]string{sub, "--quiet"}, variant.flags...)
					return runCLI(t, "", append(args, path)...)
				}
				cOut, cErr, cCode := run("convert")
				fOut, fErr, fCode := run("filter")
				if cCode != fCode {
					t.Errorf("exit code: convert %d, filter %d\nconvert stderr:\n%s\nfilter stderr:\n%s", cCode, fCode, cErr, fErr)
				}
				if cOut != fOut {
					t.Errorf("output differs\n--- first divergence ---\n%s", firstDiff([]byte(fOut), []byte(cOut)))
				}
				if cErr != fErr {
					t.Errorf("diagnostics differ\nconvert:\n%s\nfilter:\n%s", indent(cErr), indent(fErr))
				}
			})
		}
	}
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n  ")
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

// TestExecFailsWhenADestinationCannotClose: a destination's close error
// (a buffered destination flushes at close) is logged and fails the
// run, even though every game succeeded; the summary still counts the
// games.
func TestExecFailsWhenADestinationCannotClose(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("1. e4 *\n"))
	var log bytes.Buffer
	r := &run{
		cmd:   cmd,
		logW:  &log,
		tags:  output.NewTagRegistry(),
		pipe:  pipeline.New(pipeline.Config{}),
		route: routeNowhere,
	}
	r.closers = append(r.closers, closerFunc(func() error { return errors.New("out.pgn: flush: no space left on device") }))
	err := r.exec(nil)
	if !errors.Is(err, errProblems) {
		t.Errorf("exec = %v, want errProblems", err)
	}
	if want := "out.pgn: flush: no space left on device\n1 game matched out of 1.\n"; log.String() != want {
		t.Errorf("log = %q, want %q", log.String(), want)
	}
	r.close()
}
