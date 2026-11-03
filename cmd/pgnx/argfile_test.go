package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExpandArgsReadsFlagsValuesAndInputsFromFile(t *testing.T) {
	file := writeTemp(t, "opts.txt", strings.Join([]string{
		"% Find a supported Knight on e4.",
		`--tag FENPattern "*/*/*/*/???pN???/???P????/*/*"`,
		"",
		"   % an indented comment, after a blank line",
		"  --mark-matches   HERE  ",
		"--add-label-tag",
		"--line-width=60",
		"-o out.pgn",
		"My Games.pgn",
	}, "\r\n"))

	got, err := expandArgs([]string{"filter", "@" + file, "more.pgn"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"filter",
		"--tag", `FENPattern "*/*/*/*/???pN???/???P????/*/*"`,
		"--mark-matches", "HERE",
		"--add-label-tag",
		"--line-width=60",
		"-o", "out.pgn",
		"My Games.pgn",
		"more.pgn"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expandArgs:\n got %q\nwant %q", got, want)
	}
}

func TestExpandArgsNestsFilesAndStopsAtDoubleDash(t *testing.T) {
	inner := writeTemp(t, "inner.txt", "--no-tags\n")
	outer := writeTemp(t, "outer.txt", "@"+inner+"\n--quiet\n")

	got, err := expandArgs([]string{"convert", "@" + outer, "--", "@literal.pgn"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"convert", "--no-tags", "--quiet", "--", "@literal.pgn"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expandArgs:\n got %q\nwant %q", got, want)
	}
}

func TestExpandArgsErrors(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "self.txt")
	if err := os.WriteFile(self, []byte("--quiet\n@"+self+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"missing", []string{"@" + filepath.Join(dir, "nope.txt")}, "nope.txt"},
		{"cycle", []string{"@" + self}, "includes itself"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := expandArgs(c.args)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("expandArgs(%q) error = %v, want one mentioning %q", c.args, err, c.want)
			}
		})
	}
}

// TestArgumentFileMatchesDirectFlags runs the shape of the C repo's test-AA
// argslist.txt (everything in the file: the subcommand, output, tag
// criteria and input files; the -d duplicates file is left out until
// duplicate detection is ported) and requires the output to be
// byte-identical to the same run with the flags given directly.
func TestArgumentFileMatchesDirectFlags(t *testing.T) {
	dir := t.TempDir()
	fischer := filepath.Join("testdata", "golden", "infiles", "fischer.pgn")
	petrosian := filepath.Join("testdata", "golden", "infiles", "petrosian.pgn")

	direct := filepath.Join(dir, "direct.pgn")
	_, stderr, code := runCLI(t, "", "filter", "--no-variations", "--quiet",
		"--output", direct, "--tag", `White "Fischer"`, "--tag", `Black "Petrosian"`,
		fischer, petrosian)
	if code != 0 {
		t.Fatalf("direct run: exit %d\n%s", code, stderr)
	}

	viaFile := filepath.Join(dir, "viafile.pgn")
	file := writeTemp(t, "argslist.txt", strings.Join([]string{
		"% Strip variations.",
		"filter",
		"--no-variations",
		"% Silent mode",
		"--quiet",
		"% Unique file.",
		"--output " + viaFile,
		"% Tag criteria",
		`--tag White "Fischer"`,
		`--tag Black "Petrosian"`,
		"% The game files.",
		fischer,
		petrosian,
	}, "\n"))
	_, stderr, code = runCLI(t, "", "@"+file)
	if code != 0 {
		t.Fatalf("args-file run: exit %d\n%s", code, stderr)
	}

	want, err := os.ReadFile(direct)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(viaFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(want) == 0 {
		t.Fatal("direct run selected no games; the test proves nothing")
	}
	if string(got) != string(want) {
		t.Errorf("args-file output differs from the direct run\n got %d bytes\nwant %d bytes", len(got), len(want))
	}
}

func TestArgumentFileErrorIsUsageError(t *testing.T) {
	_, stderr, code := runCLI(t, "", "convert", "@"+filepath.Join(t.TempDir(), "missing.txt"))
	if code != 1 {
		t.Errorf("exit %d, want 1 (usage error)", code)
	}
	if !strings.Contains(stderr, "pgnx: argument file") || !strings.Contains(stderr, "missing.txt") {
		t.Errorf("stderr %q should name the argument file", stderr)
	}
}
