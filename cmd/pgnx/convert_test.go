package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runCLI(t *testing.T, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	// execute maps our sentinel errors to 2 and usage errors to 1 (via
	// exitCode), and prints non-sentinel errors to the error stream —
	// the same path main() uses.
	code = execute(root)
	return out.String(), errb.String(), code
}

const fools = "1. f3 e5 2. g4 Qh4# 0-1\n"

func TestConvertStdinToStdout(t *testing.T) {
	out, _, code := runCLI(t, fools, "convert", "--quiet")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "Qh4# 0-1") {
		t.Errorf("output: %q", out)
	}
}

func TestConvertToFile(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pgn")
	outFile := filepath.Join(dir, "out.pgn")
	os.WriteFile(in, []byte(fools), 0o644)
	_, _, code := runCLI(t, "", "convert", "--quiet", "-o", outFile, in)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	data, _ := os.ReadFile(outFile)
	if !strings.Contains(string(data), "1. f3 e5") {
		t.Errorf("file: %q", data)
	}
}

func TestConvertBrokenGameSkippedExit2(t *testing.T) {
	out, errOut, code := runCLI(t, "1. e4 e4 *\n\n"+fools, "convert", "--quiet")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(out, "Qh4#") || strings.Contains(out, "e4 e4") {
		t.Errorf("good game kept, bad skipped; out=%q", out)
	}
	if !strings.Contains(errOut, "game 1") {
		t.Errorf("stderr should identify broken game: %q", errOut)
	}
}

func TestConvertFormatFlag(t *testing.T) {
	out, _, _ := runCLI(t, fools, "convert", "--quiet", "-f", "uci")
	if !strings.Contains(out, "f2f3 e7e5 g2g4 d8h4") {
		t.Errorf("uci out: %q", out)
	}
}

func TestUnknownFlagErrorPrinted(t *testing.T) {
	_, errOut, code := runCLI(t, "", "convert", "--badflag")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "badflag") {
		t.Errorf("stderr should name the unknown flag: %q", errOut)
	}
}

func TestInvalidFormatErrorPrinted(t *testing.T) {
	_, errOut, code := runCLI(t, fools, "convert", "-f", "bogus")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "bogus") {
		t.Errorf("stderr should mention the bad format value: %q", errOut)
	}
}

func TestBrokenGameNoDuplicateSentinelLine(t *testing.T) {
	_, errOut, code := runCLI(t, "1. e4 e4 *\n", "convert", "--quiet")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if strings.Contains(errOut, errProblems.Error()) {
		t.Errorf("sentinel must not be printed on top of per-game errors: %q", errOut)
	}
}

func TestSummaryLine(t *testing.T) {
	_, errOut, _ := runCLI(t, fools, "convert")
	if !strings.Contains(errOut, "1 game matched out of 1.") {
		t.Errorf("summary: %q", errOut)
	}
}

// pgn-extract's consistent_FEN_tags runs on every game, not only when
// filtering: a FEN game without a SetUp tag gains SetUp "1" (seen on TWIC
// issues 1267, 1393 and 1426), and SetUp "1" without a FEN is dropped.
func TestConvertAddsSetUpToFENGame(t *testing.T) {
	const in = "[Event \"a\"]\n[Result \"*\"]\n[FEN \"3n2r1/5p1k/R1pp1p2/1p2p2p/1P2P1P1/1BPP3P/5b1N/7K w - - 1 44\"]\n\n44. Ra1 Ne6 *\n"
	out, _, code := runCLI(t, in, "convert", "--quiet")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "[SetUp \"1\"]\n[FEN \"3n2r1") {
		t.Errorf("SetUp \"1\" should precede the FEN tag; out=%q", out)
	}
}

func TestConvertDropsSetUpWithoutFEN(t *testing.T) {
	out, errOut, code := runCLI(t, "[Event \"a\"]\n[SetUp \"1\"]\n\n1. e4 *\n\n"+fools, "convert", "--quiet")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if strings.Contains(out, "1. e4 *") || !strings.Contains(out, "Qh4#") {
		t.Errorf("inconsistent game dropped, good game kept; out=%q", out)
	}
	if !strings.Contains(errOut, "game 1") || !strings.Contains(errOut, "SetUp") {
		t.Errorf("stderr should name the game and the tag: %q", errOut)
	}
}

// --piece-letters is pgn-extract's -Wsan<letters> suffix: the alphabet
// san, elalg and xlalg output spells pieces with. Oracle:
//
//	printf '1. f3 e5 2. g4 Qh4# 0-1\n' | pgn-extract -WsanBSLTDK --quiet  -> 1. f3 e5 2. g4 Dh4# 0-1
func TestConvertPieceLetters(t *testing.T) {
	for _, c := range []struct {
		format, want string
	}{
		{"san", "1. f3 e5 2. g4 Dh4# 0-1"},
		{"elalg", "1. f2f3 e7e5 2. g2g4 Dd8h4# 0-1"},
		{"xlalg", "1. f2-f3 e7-e5 2. g2-g4 Dd8-h4# 0-1"},
	} {
		out, errOut, code := runCLI(t, fools, "convert", "--quiet", "-f", c.format, "--piece-letters", "BSLTDK")
		if code != 0 {
			t.Fatalf("-f %s: exit %d (%s)", c.format, code, errOut)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("-f %s: output %q, want line %q", c.format, out, c.want)
		}
	}
	// filter and split share the output flags.
	out, _, code := runCLI(t, fools, "filter", "--quiet", "--checkmate", "--piece-letters", "BSLTDK")
	if code != 0 || !strings.Contains(out, "Dh4#") {
		t.Errorf("filter: exit %d, output %q", code, out)
	}
}

// A malformed alphabet, or one given for a format that cannot spell
// pieces with it, is a usage error (exit 1). The C tool exits 1 on the
// former and silently ignores -Wlalg<letters>; pgnx refuses both.
func TestConvertPieceLettersUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--piece-letters", "BSLTD"},
		{"--piece-letters", "BSLTDKX"},
		{"--piece-letters", "P+"},
		{"--piece-letters", "BSLTDK", "-f", "lalg"},
		{"--piece-letters", "BSLTDK", "-f", "halg"},
		{"--piece-letters", "BSLTDK", "-f", "uci"},
		{"--piece-letters", "BSLTDK", "-f", "epd"},
		{"--piece-letters", "BSLTDK", "-f", "source"},
	} {
		_, errOut, code := runCLI(t, fools, append([]string{"convert", "--quiet"}, args...)...)
		if code != 1 || !strings.Contains(errOut, "piece-letters") {
			t.Errorf("%v: exit %d, want 1 naming the flag (%s)", args, code, errOut)
		}
	}
}
