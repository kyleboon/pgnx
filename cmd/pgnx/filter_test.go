package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const twoGames = "[White \"Fischer\"]\n[Black \"Spassky\"]\n\n1. e4 e5 *\n\n[White \"Petrosian\"]\n[Black \"Tal\"]\n\n1. d4 d5 *\n"

func TestFilterNoCriteriaMatchesConvert(t *testing.T) {
	want, _, code := runCLI(t, twoGames, "convert", "--quiet")
	if code != 0 {
		t.Fatalf("convert exit %d", code)
	}
	got, errOut, code := runCLI(t, twoGames, "filter")
	if code != 0 {
		t.Fatalf("filter exit %d: %s", code, errOut)
	}
	if got != want {
		t.Errorf("filter without criteria differs from convert:\n%s\n---\n%s", got, want)
	}
	if !strings.Contains(errOut, "2 games matched out of 2.") {
		t.Errorf("summary: %q", errOut)
	}
}

func TestFilterPlayerRoutesMatchedAndUnmatched(t *testing.T) {
	dir := t.TempDir()
	matched := filepath.Join(dir, "m.pgn")
	unmatched := filepath.Join(dir, "u.pgn")
	_, errOut, code := runCLI(t, twoGames, "filter", "--player", "Fischer", "-o", matched, "--unmatched", unmatched)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	m, _ := os.ReadFile(matched)
	u, _ := os.ReadFile(unmatched)
	if !strings.Contains(string(m), "Fischer") || strings.Contains(string(m), "Petrosian") {
		t.Errorf("matched: %s", m)
	}
	if !strings.Contains(string(u), "Petrosian") || strings.Contains(string(u), "Fischer") {
		t.Errorf("unmatched: %s", u)
	}
	if !strings.Contains(errOut, "1 game matched out of 2.") {
		t.Errorf("summary: %q", errOut)
	}
}

func TestFilterUnmatchedToStdout(t *testing.T) {
	out, _, code := runCLI(t, twoGames, "filter", "--quiet", "--player", "Fischer", "--suppress-matched", "--unmatched", "-")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "Petrosian") || strings.Contains(out, "Fischer") {
		t.Errorf("stdout: %s", out)
	}
}

func TestFilterSuppressMatchedLeavesStdoutEmpty(t *testing.T) {
	out, errOut, code := runCLI(t, twoGames, "filter", "--player", "Fischer", "--suppress-matched")
	if code != 0 || out != "" {
		t.Errorf("exit %d stdout %q", code, out)
	}
	if strings.Contains(errOut, "matched out of") {
		t.Errorf("summary should be suppressed: %q", errOut)
	}
}

func TestFilterStopAfterStopsReading(t *testing.T) {
	// The second game is unparsable (unterminated comment); --stop-after 1
	// must stop before the scanner reaches it.
	in := "[White \"Fischer\"]\n\n1. e4 *\n\n[White \"Fischer\"]\n\n1. e4 {never closed\n"
	out, errOut, code := runCLI(t, in, "filter", "--quiet", "--player", "Fischer", "--stop-after", "1")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if strings.Count(out, "[White") != 1 {
		t.Errorf("want exactly one game: %s", out)
	}
	_, _, code = runCLI(t, in, "filter", "--quiet", "--player", "Fischer")
	if code != 2 {
		t.Errorf("without --stop-after the bad game is reached: exit %d", code)
	}
}

func TestFilterStopAfterAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.pgn")
	b := filepath.Join(dir, "b.pgn")
	os.WriteFile(a, []byte("[White \"Fischer\"]\n\n1. e4 *\n"), 0o644)
	os.WriteFile(b, []byte("[White \"Fischer\"]\n\n1. d4 *\n"), 0o644)
	out, _, code := runCLI(t, "", "filter", "--quiet", "--stop-after", "1", a, b)
	if code != 0 || strings.Contains(out, "d4") {
		t.Errorf("second file should not be read: exit %d out %s", code, out)
	}
}

func TestFilterConflictingFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--checkmate", "--stalemate"},
		{"--fifty", "--seventy-five"},
		{"--higher-rated-winner", "--lower-rated-winner"},
		{"--no-setup-tags", "--only-setup-tags"},
		{"--moves", "0"},
		{"--min-moves", "10", "--max-moves", "5"},
		{"--select-only", "3,2"},
		{"--tag", "White Fischer"},
		{"--material", "kx k"},
		{"--repetition=-1"},
		{"--first-game", "5", "--game-limit", "2"},
	} {
		_, errOut, code := runCLI(t, fools, append([]string{"filter"}, args...)...)
		if code != 1 {
			t.Errorf("%v: exit %d, want 1 (%s)", args, code, errOut)
		}
	}
}

func TestFilterNoBadResultsExit2(t *testing.T) {
	in := "[Result \"1-0\"]\n\n1. e4 0-1\n\n[Result \"1-0\"]\n\n1. e4 1-0\n"
	out, errOut, code := runCLI(t, in, "filter", "--quiet", "--no-bad-results")
	if code != 2 {
		t.Fatalf("exit %d", code)
	}
	if strings.Count(out, "[Result") != 1 || !strings.Contains(errOut, "game 1") {
		t.Errorf("out %q err %q", out, errOut)
	}
}

func TestFilterTagFileMissing(t *testing.T) {
	_, errOut, code := runCLI(t, fools, "filter", "--tag-file", filepath.Join(t.TempDir(), "nope.txt"))
	if code != 1 || !strings.Contains(errOut, "nope.txt") {
		t.Errorf("exit %d err %q", code, errOut)
	}
}

func TestFilterTagFileAndRepetitionFlag(t *testing.T) {
	dir := t.TempDir()
	tf := filepath.Join(dir, "t.txt")
	os.WriteFile(tf, []byte("% criteria\nWhite \"Fischer\"\r\n"), 0o644)
	out, _, code := runCLI(t, twoGames, "filter", "--quiet", "--tag-file", tf)
	if code != 0 || strings.Contains(out, "Petrosian") || !strings.Contains(out, "Fischer") {
		t.Errorf("tag-file: exit %d out %s", code, out)
	}

	rep := "1. Nf3 Nf6 2. Ng1 Ng8 3. Nf3 Nf6 4. Ng1 Ng8 *\n"
	out, _, code = runCLI(t, rep, "filter", "--quiet", "--repetition", "--mark-matches", "R")
	if code != 0 || !strings.Contains(out, "{ R }") {
		t.Errorf("--repetition: exit %d out %s", code, out)
	}
	out, _, code = runCLI(t, rep, "filter", "--quiet", "--repetition=5")
	if code != 0 || out != "" {
		t.Errorf("--repetition=5: exit %d out %q", code, out)
	}
}

func TestFilterKeepBrokenEvaluated(t *testing.T) {
	in := "[White \"Fischer\"]\n\n1. e4 e4 *\n"
	out, _, code := runCLI(t, in, "filter", "--quiet", "--keep-broken", "--player", "Fischer")
	if code != 2 || !strings.Contains(out, "e4 e4") {
		t.Errorf("kept broken game: exit %d out %q", code, out)
	}
	out, _, code = runCLI(t, in, "filter", "--quiet", "--keep-broken", "--player", "Tal", "--unmatched", "-")
	if code != 2 || !strings.Contains(out, "e4 e4") {
		t.Errorf("kept broken game to unmatched: exit %d out %q", code, out)
	}
}

func TestFilterUsageErrorsPrinted(t *testing.T) {
	_, errOut, code := runCLI(t, fools, "filter", "--checkmate", "--stalemate")
	if code != 1 || !strings.Contains(errOut, "checkmate") {
		t.Errorf("exit %d err %q", code, errOut)
	}
}
