package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
