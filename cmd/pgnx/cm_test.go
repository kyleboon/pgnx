package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCMConformance pins -f cm against oracles the C binary produced over
// the golden corpus (the C test suite has no -Wcm target). Each oracle
// was made from ~/code/pgn-extract with the command in its comment.
func TestCMConformance(t *testing.T) {
	cases := []struct {
		name   string
		input  string // under testdata/golden/infiles
		oracle string // under testdata/cm
	}{
		// pgn-extract -Wcm -otest-ucW-cm-out.txt --quiet test-ucW.pgn
		{"test-ucW", "test-ucW.pgn", "test-ucW-cm-out.txt"},
		// pgn-extract -Wcm -otest-C-cm-out.txt --quiet test-C.pgn
		{"test-C", "test-C.pgn", "test-C-cm-out.txt"},
		// pgn-extract -Wcm -ofischer-cm-out.txt --quiet fischer.pgn
		{"fischer", "fischer.pgn", "fischer-cm-out.txt"},
		// pgn-extract -Wcm -onajdorf-cm-out.txt --quiet najdorf.pgn
		{"najdorf", "najdorf.pgn", "najdorf-cm-out.txt"},
		// pgn-extract -Wcm -otest-setup-in-cm-out.txt --quiet test-setup-in.pgn
		{"test-setup-in", "test-setup-in.pgn", "test-setup-in-cm-out.txt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			outFile := filepath.Join(t.TempDir(), "out.cm")
			args := []string{"convert", "-f", "cm", "--quiet", "-o", outFile,
				filepath.Join("testdata", "golden", "infiles", c.input)}
			_, errOut, code := runCLI(t, "", args...)
			if code != 0 {
				t.Fatalf("pgnx %v: exit %d\n%s", args, code, errOut)
			}
			got, err := os.ReadFile(outFile)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", "cm", c.oracle))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("output differs from C oracle %s\n--- first divergence ---\n%s", c.oracle, firstDiff(got, want))
			}
		})
	}
}

// TestCMUsageErrors: the C tool drops --json with a warning under -Wcm
// and silently ignores a -Wcm piece-letter suffix; pgnx refuses both.
func TestCMUsageErrors(t *testing.T) {
	cases := []struct {
		args []string
		flag string
	}{
		{[]string{"convert", "--json", "-f", "cm"}, "--json"},
		{[]string{"filter", "--json", "-f", "cm"}, "--json"},
		{[]string{"convert", "-f", "cm", "--piece-letters", "BSLTDK"}, "--piece-letters"},
	}
	for _, c := range cases {
		_, errOut, code := runCLI(t, fools, c.args...)
		if code != 1 {
			t.Errorf("%v: exit %d, want 1", c.args, code)
		}
		if !strings.Contains(errOut, c.flag) {
			t.Errorf("%v: stderr %q does not name %s", c.args, errOut, c.flag)
		}
	}
}

// TestCMFENStartWarns: the C tool writes a game from a FEN in CM format
// anyway, with "Unable to output CM games other than from the starting
// position." on its log; pgnx warns too, without failing the game.
func TestCMFENStartWarns(t *testing.T) {
	src := "[FEN \"4k3/8/8/8/8/8/4P3/4K3 w - - 0 40\"]\n[SetUp \"1\"]\n[Result \"*\"]\n\n40. e4 Kd7 *\n"
	out, errOut, code := runCLI(t, src, "convert", "-f", "cm")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, " 40. e4             Kd7            ; incomplete result\n") {
		t.Errorf("stdout:\n%s", out)
	}
	if !strings.Contains(errOut, "starting position") {
		t.Errorf("stderr %q does not warn about the starting position", errOut)
	}
}
