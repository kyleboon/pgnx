package main

import (
	"strings"
	"testing"
)

// TestVersionFlag verifies the spec-mandated "pgnx --version" prints a
// version string and exits 0.
func TestVersionFlag(t *testing.T) {
	out, _, code := runCLI(t, "", "--version")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, version) {
		t.Errorf("output should contain version %q, got %q", version, out)
	}
}
