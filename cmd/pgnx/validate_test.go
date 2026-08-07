package main

import (
	"strings"
	"testing"
)

func TestValidateGoodAndBad(t *testing.T) {
	input := "1. e4 e4 *\n\n1. f3 e5 2. g4 Qh4# 0-1\n"
	out, _, code := runCLI(t, input, "validate")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(out, "game 1") {
		t.Errorf("out: %q", out)
	}
	out2, _, code2 := runCLI(t, input, "validate", "--summary")
	if code2 != 2 || !strings.Contains(out2, "1 of 2 games valid") {
		t.Errorf("summary out: %q code %d", out2, code2)
	}
	_, _, code3 := runCLI(t, "1. e4 e5 *\n", "validate")
	if code3 != 0 {
		t.Errorf("clean input exit = %d", code3)
	}
}

func TestInfo(t *testing.T) {
	out, _, code := runCLI(t, "1. e4 e5 *\n\n1. f3 e5 2. g4 Qh4# 0-1\n", "info")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{"games:   2", "plies:   6 (avg 3.0)", "0-1 1", "* 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("info missing %q in:\n%s", want, out)
		}
	}
}
