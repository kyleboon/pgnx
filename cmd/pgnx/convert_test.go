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
	err := root.Execute()
	code = 0
	if err != nil {
		code = exitCode(err) // helper mapping our sentinel errors to 2, usage errors to 1
	}
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

func TestSummaryLine(t *testing.T) {
	_, errOut, _ := runCLI(t, fools, "convert")
	if !strings.Contains(errOut, "1 game matched out of 1.") {
		t.Errorf("summary: %q", errOut)
	}
}
