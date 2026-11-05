package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	gameA = "[White \"A\"]\n\n1. e4 e5 2. Nf3 Nc6 3. Bb5 *\n"
	gameB = "[White \"B\"]\n\n1. d4 d5 2. c4 e6 *\n"
	// gameA2 repeats gameA's moves under another name.
	gameA2 = "[White \"A2\"]\n\n1. e4 e5 2. Nf3 Nc6 3. Bb5 *\n"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func TestDedupeDropsDuplicatesByDefault(t *testing.T) {
	out, errOut, code := runCLI(t, gameA+gameB+gameA2, "dedupe")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, `[White "A"]`) || !strings.Contains(out, `[White "B"]`) || strings.Contains(out, `[White "A2"]`) {
		t.Errorf("output:\n%s", out)
	}
	// A dropped duplicate is unmatched (grammar.c: game_matches stays
	// false), so the summary does not count it.
	if !strings.Contains(errOut, "2 games matched out of 3.") {
		t.Errorf("summary: %q", errOut)
	}
}

func TestDedupeUnmatchedReceivesDroppedDuplicates(t *testing.T) {
	dir := t.TempDir()
	unm := filepath.Join(dir, "u.pgn")
	out, _, code := runCLI(t, gameA+gameB+gameA2, "dedupe", "--quiet", "--unmatched", unm)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(out, `"A2"`) {
		t.Errorf("duplicate on stdout:\n%s", out)
	}
	if u := readFile(t, unm); !strings.Contains(u, `[White "A2"]`) || strings.Contains(u, `[White "B"]`) {
		t.Errorf("unmatched:\n%s", u)
	}
}

func TestDedupeDuplicatesFileWithOriginComments(t *testing.T) {
	dir := t.TempDir()
	a := writeTemp(t, "a.pgn", gameA+gameB)
	b := writeTemp(t, "b.pgn", gameA2+gameB+gameA)
	out := filepath.Join(dir, "out.pgn")
	dupes := filepath.Join(dir, "dupes.pgn")
	_, errOut, code := runCLI(t, "", "dedupe", "-o", out, "--duplicates", dupes, a, b)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	// Diverted duplicates are matched games (they would have been
	// written), so the summary counts all five.
	if !strings.Contains(errOut, "5 games matched out of 5.") {
		t.Errorf("summary: %q", errOut)
	}
	if o := readFile(t, out); strings.Count(o, "[White ") != 2 || strings.Contains(o, "First found in") {
		t.Errorf("output:\n%s", o)
	}
	// grammar.c: "{ From: <input> }" once per input file contributing
	// duplicates, "{ First found in: <origin> }" before each duplicate.
	roster := func(white, moves string) string {
		return "[Event \"?\"]\n[Site \"?\"]\n[Date \"????.??.??\"]\n[Round \"?\"]\n[White \"" + white + "\"]\n[Black \"?\"]\n[Result \"*\"]\n\n" + moves + "\n\n"
	}
	want := "{ From: " + b + " }\n" +
		"{ First found in: " + a + " }\n" + roster("A2", "1. e4 e5 2. Nf3 Nc6 3. Bb5 *") +
		"{ First found in: " + a + " }\n" + roster("B", "1. d4 d5 2. c4 e6 *") +
		"{ First found in: " + a + " }\n" + roster("A", "1. e4 e5 2. Nf3 Nc6 3. Bb5 *")
	if d := readFile(t, dupes); d != want {
		t.Errorf("duplicates file:\n%s\nwant:\n%s", d, want)
	}
}

func TestDedupeNoCommentsSuppressesOriginComments(t *testing.T) {
	dir := t.TempDir()
	dupes := filepath.Join(dir, "dupes.pgn")
	_, _, code := runCLI(t, gameA+gameA2, "dedupe", "--quiet", "--no-comments", "-o", filepath.Join(dir, "o.pgn"), "--duplicates", dupes)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if d := readFile(t, dupes); strings.Contains(d, "{") || !strings.Contains(d, `"A2"`) {
		t.Errorf("duplicates file:\n%s", d)
	}
}

func TestDedupeStdinOriginIsUnderscored(t *testing.T) {
	// hashing.c: "Without a filename, suppressing duplicates on stdin
	// does not work", so the origin of a stdin game is "_stdin_"; the
	// From line names the input as the lexer does, "stdin".
	dir := t.TempDir()
	dupes := filepath.Join(dir, "dupes.pgn")
	_, _, code := runCLI(t, gameA+gameA2, "dedupe", "--quiet", "-o", filepath.Join(dir, "o.pgn"), "--duplicates", dupes)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if d := readFile(t, dupes); !strings.HasPrefix(d, "{ From: stdin }\n{ First found in: _stdin_ }\n") {
		t.Errorf("duplicates file:\n%s", d)
	}
}

func TestDedupeOnlyDuplicates(t *testing.T) {
	out, errOut, code := runCLI(t, gameA+gameB+gameA2, "dedupe", "--only-duplicates")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if strings.Count(out, "[White ") != 1 || !strings.Contains(out, `"A2"`) {
		t.Errorf("output:\n%s", out)
	}
	if !strings.Contains(errOut, "1 game matched out of 3.") {
		t.Errorf("summary: %q", errOut)
	}

	// With --duplicates too, the duplicates go there and the output gets
	// nothing (the C tool allows -U with -d).
	dir := t.TempDir()
	outFile := filepath.Join(dir, "out.pgn")
	dupes := filepath.Join(dir, "dupes.pgn")
	_, _, code = runCLI(t, gameA+gameB+gameA2, "dedupe", "--quiet", "--no-comments", "--only-duplicates", "-o", outFile, "--duplicates", dupes)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if o := readFile(t, outFile); o != "" {
		t.Errorf("output not empty:\n%s", o)
	}
	if d := readFile(t, dupes); strings.Count(d, "[White ") != 1 || !strings.Contains(d, `"A2"`) {
		t.Errorf("duplicates:\n%s", d)
	}
}

func TestDedupeComposesWithCriteriaAndSelector(t *testing.T) {
	// Only matched games are visited: a game failing --white never
	// registers, so the later identical game is a first occurrence.
	out, errOut, code := runCLI(t, gameA+gameB+gameA2, "dedupe", "--white", "A2", "--white", "B")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, `"A2"`) || !strings.Contains(out, `"B"`) || strings.Contains(out, `[White "A"]`) {
		t.Errorf("output:\n%s", out)
	}
	// The visit precedes the Selector: a duplicate held back by
	// --first-game is still registered... and a first occurrence before
	// --first-game is registered too, so its later copy is a duplicate.
	out, _, code = runCLI(t, gameA+gameB+gameA2, "dedupe", "--quiet", "--first-game", "2")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Count(out, "[White ") != 1 || !strings.Contains(out, `"B"`) {
		t.Errorf("--first-game 2 output:\n%s", out)
	}
}

func TestDedupeUsageErrors(t *testing.T) {
	dir := t.TempDir()
	same := filepath.Join(dir, "x.pgn")
	for _, args := range [][]string{
		{"dedupe", "-o", same, "--duplicates", same},
		{"dedupe", "--unmatched", same, "--duplicates", same},
	} {
		_, errOut, code := runCLI(t, gameA, args...)
		if code != 1 || !strings.Contains(errOut, "--duplicates") {
			t.Errorf("%v: exit %d, stderr %q", args, code, errOut)
		}
	}
}
