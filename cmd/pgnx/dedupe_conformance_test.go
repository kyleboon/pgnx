package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// dedupeGoldenCases mirrors the C Makefile's duplicate-detection targets,
// which take two input files and write an output and, for some, a
// duplicates file; the goldenCases table can't express either. Each row
// keeps the C command as a comment, as goldenCases does. Paths in args
// are relative to testdata/golden/infiles; the runner appends --quiet,
// -o and the inputs, and --duplicates when dupesOracle is set.
var dedupeGoldenCases = []struct {
	name        string
	args        []string
	inputs      []string
	oracle      string // the -o file
	dupesOracle string // the -d file; "" when the row writes none
	checkFiles  []string
}{
	// test-duplicates: pgn-extract -C -dtest-d-dupes.pgn -otest-d-unique.pgn --quiet fischer.pgn petrosian.pgn
	{"test-duplicates", []string{"--no-comments"}, []string{"fischer.pgn", "petrosian.pgn"}, "test-d-unique.pgn", "test-d-dupes.pgn", nil},
	// test-noduplicates: pgn-extract -D -otest-DD-unique.pgn --quiet fischer.pgn petrosian.pgn
	// (and the same with --noduplicates). Dropping duplicates is dedupe's
	// default disposition.
	{"test-noduplicates", nil, []string{"fischer.pgn", "petrosian.pgn"}, "test-DD-unique.pgn", "", nil},
	// test-nounique: pgn-extract -U -otest-UU-unique.pgn --quiet fischer.pgn petrosian.pgn
	// (and the same with --nounique)
	{"test-nounique", []string{"--only-duplicates"}, []string{"fischer.pgn", "petrosian.pgn"}, "test-UU-unique.pgn", "", nil},
	// test-ZZ: pgn-extract -C -Z -dtest-ZZ-dupes.pgn -otest-ZZ-unique.pgn --quiet fischer.pgn petrosian.pgn
	// -Z (the on-disk hash table) is not ported: pgnx's seen-set is in
	// memory. The oracles are byte-identical to test-duplicates', so the
	// row is the same command (CONFORMANCE.md).
	{"test-ZZ", []string{"--no-comments"}, []string{"fischer.pgn", "petrosian.pgn"}, "test-ZZ-unique.pgn", "test-ZZ-dupes.pgn", nil},
	// test-checkfile: pgn-extract -cpetrosian.pgn -D -TpPetrosian -otest-c-out.pgn --quiet fischer.pgn
	// and then the same with -cclist.txt, a list file naming
	// petrosian.pgn, into the same oracle. The list-by-suffix form of -c
	// is not ported (an argument file does it; MIGRATION.md), so the
	// second run is this same command.
	{"test-checkfile", []string{"--player", "Petrosian"}, []string{"fischer.pgn"}, "test-c-out.pgn", "", []string{"petrosian.pgn"}},
}

func TestGoldenConformanceDedupe(t *testing.T) {
	for _, c := range dedupeGoldenCases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			outFile := filepath.Join(dir, "out.pgn")
			dupesFile := filepath.Join(dir, "dupes.pgn")
			args := append([]string{"dedupe", "--quiet", "-o", outFile}, c.args...)
			if c.dupesOracle != "" {
				args = append(args, "--duplicates", dupesFile)
			}
			for _, cf := range c.checkFiles {
				args = append(args, "--check-file", filepath.Join("testdata", "golden", "infiles", cf))
			}
			for _, in := range c.inputs {
				args = append(args, filepath.Join("testdata", "golden", "infiles", in))
			}
			root := NewRootCmd()
			var errb bytes.Buffer
			root.SetErr(&errb)
			if err := runRoot(root, args); err != nil && !errors.Is(err, errProblems) {
				t.Fatalf("pgnx %v: %v\n%s", args, err, errb.String())
			}
			compareOracle(t, outFile, c.oracle)
			if c.dupesOracle != "" {
				compareOracle(t, dupesFile, c.dupesOracle)
			}
		})
	}
}

// compareOracle requires got, a file pgnx wrote, to byte-match the C
// oracle file under testdata/golden/outfiles.
func compareOracle(t *testing.T, got, oracle string) {
	t.Helper()
	g, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "golden", "outfiles", oracle))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(g, want) {
		t.Errorf("%s differs from C oracle %s\n--- first divergence ---\n%s", filepath.Base(got), oracle, firstDiff(g, want))
	}
}
