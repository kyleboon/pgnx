package main

import (
	"strings"
	"testing"
)

// TestScannerFatalErrorSurfaced verifies that a fatal scanner error (e.g.
// an unterminated comment) occurring after one or more good games is not
// silently swallowed: it must be visible on the I/O-error writer (stderr
// by default) and must cause the command to exit 2, matching the exit
// code used for any other "one or more games failed" condition. Partial
// good output (games parsed before the fatal error) must still be
// emitted.
func TestScannerFatalErrorSurfaced(t *testing.T) {
	input := "1. f3 e5 2. g4 Qh4# 0-1\n\n1. e4 e5 {unterminated\n"

	t.Run("convert", func(t *testing.T) {
		out, errOut, code := runCLI(t, input, "convert", "--quiet")
		if code != 2 {
			t.Fatalf("exit = %d, want 2; stderr=%q", code, errOut)
		}
		if !strings.Contains(out, "Qh4#") {
			t.Errorf("partial good output missing from stdout: %q", out)
		}
		if !strings.Contains(errOut, "unterminated") {
			t.Errorf("fatal scanner error not visible on stderr: %q", errOut)
		}
	})

	t.Run("validate", func(t *testing.T) {
		_, errOut, code := runCLI(t, input, "validate")
		if code != 2 {
			t.Fatalf("exit = %d, want 2; stderr=%q", code, errOut)
		}
		if !strings.Contains(errOut, "unterminated") {
			t.Errorf("fatal scanner error not visible on stderr: %q", errOut)
		}
	})
}
