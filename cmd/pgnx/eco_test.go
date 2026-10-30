package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goldenECO is the C repo's eco.pgn, vendored for the eco package tests.
// It is absolute so tests that change directory can still reach it.
var goldenECO = func() string {
	p, err := filepath.Abs(filepath.Join("testdata", "golden", "eco.pgn"))
	if err != nil {
		panic(err)
	}
	return p
}()

func TestConvertECOFileClassifies(t *testing.T) {
	// 1. Nf3 is the Reti opening, A04 in eco.pgn; a stale Opening tag is
	// replaced along with the ECO code.
	in := "[Event \"?\"]\n[Opening \"Stale\"]\n[Result \"*\"]\n\n1. Nf3 *\n"
	out, errOut, code := runCLI(t, in, "convert", "--quiet", "--eco-file", goldenECO)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "[ECO \"A04\"]\n[Opening \"Reti opening\"]\n") || strings.Contains(out, "Stale") {
		t.Errorf("classified output:\n%s", out)
	}
}

func TestConvertECOFileSevenTagRoster(t *testing.T) {
	// Under -7 the C tool appends the ECO tags after the roster only when
	// -e is in force (output.c:1531-1539).
	in := "[Event \"?\"]\n[Annotator \"X\"]\n\n1. Nf3 *\n"
	out, _, code := runCLI(t, in, "convert", "--quiet", "--seven-tag-roster", "--eco-file", goldenECO)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "[Result \"*\"]\n[ECO \"A04\"]\n[Opening \"Reti opening\"]\n\n1. Nf3 *") || strings.Contains(out, "Annotator") {
		t.Errorf("roster output:\n%s", out)
	}
	out, _, _ = runCLI(t, in, "convert", "--quiet", "--seven-tag-roster")
	if strings.Contains(out, "ECO") {
		t.Errorf("without --eco-file the roster must not grow: %s", out)
	}
}

func TestConvertECOFileKeepBroken(t *testing.T) {
	// A broken game kept with --keep-broken is classified from the plies
	// that did replay, as the C tool classifies inside apply_move_list.
	in := "1. Nf3 d5 2. Qh5 *\n"
	out, _, code := runCLI(t, in, "convert", "--quiet", "--keep-broken", "--eco-file", goldenECO)
	if code != 2 || !strings.Contains(out, "[ECO \"A06\"]") {
		t.Errorf("exit %d out:\n%s", code, out)
	}
}

func TestConvertECOFileErrors(t *testing.T) {
	_, errOut, code := runCLI(t, fools, "convert", "--eco-file", filepath.Join(t.TempDir(), "missing.pgn"))
	if code != 1 || !strings.Contains(errOut, "missing.pgn") {
		t.Errorf("missing eco file: exit %d err %q", code, errOut)
	}
	for _, args := range [][]string{
		{"--eco-split", "0"},
		{"--eco-split", "11"},
		{"--eco-split", "1", "--output", "x.pgn"},
	} {
		_, errOut, code := runCLI(t, fools, append([]string{"convert"}, args...)...)
		if code != 1 {
			t.Errorf("%v: exit %d, want 1 (%s)", args, code, errOut)
		}
	}
}

func TestConvertECOSplit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// 1. b4 is A00; a game with no moves gets no classification and goes
	// to noeco.pgn. Two A games must land in the same file, in order.
	in := "[White \"One\"]\n\n1. b4 *\n\n[White \"Two\"]\n\n*\n\n[White \"Three\"]\n\n1. b4 Nh6 *\n"
	out, errOut, code := runCLI(t, in, "convert", "--quiet", "--eco-split", "1", "--eco-file", goldenECO)
	if code != 0 || out != "" {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
	a, err := os.ReadFile("A.pgn")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(a), "[ECO \"A00\"]") != 2 || strings.Index(string(a), "One") > strings.Index(string(a), "Three") {
		t.Errorf("A.pgn:\n%s", a)
	}
	n, err := os.ReadFile("noeco.pgn")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(n), "Two") || strings.Contains(string(n), "ECO") {
		t.Errorf("noeco.pgn:\n%s", n)
	}
	entries, _ := filepath.Glob("*.pgn")
	if len(entries) != 2 {
		t.Errorf("unexpected files: %v", entries)
	}

	// A second run appends rather than truncating (C opens with "a").
	_, _, code = runCLI(t, "1. b4 *\n", "convert", "--quiet", "--eco-split", "1", "--eco-file", goldenECO)
	a, _ = os.ReadFile("A.pgn")
	if code != 0 || strings.Count(string(a), "[ECO \"A00\"]") != 3 {
		t.Errorf("append run: exit %d A.pgn:\n%s", code, a)
	}
}

func TestConvertECOSplitWithoutECOFile(t *testing.T) {
	// -E without -e splits on the games' existing ECO tags; a non-letter
	// code goes to noeco.pgn.
	t.Chdir(t.TempDir())
	in := "[ECO \"B14\"]\n\n1. e4 *\n\n[ECO \"?\"]\n\n1. d4 *\n"
	_, errOut, code := runCLI(t, in, "convert", "--quiet", "--eco-split", "2")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	b1, err := os.ReadFile("B1.pgn")
	if err != nil || !strings.Contains(string(b1), "1. e4") {
		t.Errorf("B1.pgn: %v %s", err, b1)
	}
	n, err := os.ReadFile("noeco.pgn")
	if err != nil || !strings.Contains(string(n), "1. d4") {
		t.Errorf("noeco.pgn: %v %s", err, n)
	}
}

func TestFilterECOCriterionAfterClassification(t *testing.T) {
	// The test-Te stand-in (pgn-extract -e eco.pgn -TeB1 test-e.pgn has
	// no oracle file): an ECO criterion is tested against the classified
	// tag, so games whose source lacks an ECO tag can still match. test-e
	// classifies two games as B17.
	out, errOut, code := runCLI(t, "", "filter", "--quiet", "--eco-file", goldenECO, "--eco", "B1",
		filepath.Join("testdata", "golden", "infiles", "test-e.pgn"))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if games := strings.Count(out, "[Event "); games != 2 || strings.Count(out, "[ECO \"B17\"]") != 2 {
		t.Errorf("want the two B17 games, got %d games:\n%s", games, out)
	}
	out, _, _ = runCLI(t, "", "filter", "--quiet", "--eco", "B1",
		filepath.Join("testdata", "golden", "infiles", "test-e.pgn"))
	if out != "" {
		t.Errorf("without classification nothing carries an ECO tag: %s", out)
	}
}

func TestFilterECOSplitRoutesMatchedOnly(t *testing.T) {
	t.Chdir(t.TempDir())
	in := "[White \"Fischer\"]\n\n1. b4 *\n\n[White \"Tal\"]\n\n1. e4 *\n"
	_, errOut, code := runCLI(t, in, "filter", "--quiet", "--eco-split", "1", "--eco-file", goldenECO, "--player", "Fischer", "--unmatched", "u.pgn")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if _, err := os.Stat("B.pgn"); err == nil {
		t.Error("unmatched game must not be split into B.pgn")
	}
	a, _ := os.ReadFile("A.pgn")
	u, _ := os.ReadFile("u.pgn")
	if !strings.Contains(string(a), "Fischer") || !strings.Contains(string(u), "Tal") {
		t.Errorf("A.pgn %q u.pgn %q", a, u)
	}
}
