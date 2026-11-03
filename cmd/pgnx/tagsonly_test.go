package main

import (
	"strings"
	"testing"
)

// A tags-only game (a tag section with no movetext) is a record
// pgn-extract never emits, so every command parses with
// pgn.ParserOptions.SkipTagsOnly: it is not a game, not counted and not
// numbered (ADR 0002, CONFORMANCE.md "tags-only game").
//
// Oracle, trailing stub (emits one game, then nothing):
//
//	printf '1. d4 *\n\n[Event "stub"]\n[Result "1-0"]\n\n' | pgn-extract --quiet
//
// Oracle, stub alone (emits nothing, exits 0):
//
//	printf '[Event "stub"]\n[Result "1-0"]\n\n' | pgn-extract -7 --quiet
//
// Mid-file a stub's tags run on into the next game's tag section in both
// tools, so the stub is never a game there; see TestScannerSkipTagsOnly.
func TestTagsOnlyGameSkipped(t *testing.T) {
	const stub = "[Event \"stub\"]\n[Result \"1-0\"]\n\n"
	const real = "1. d4 *\n\n"
	const strayStub = "[Event \"stray\"]\n\n-\n"

	out, errOut, code := runCLI(t, real+stub, "convert", "--quiet")
	if code != 0 || errOut != "" {
		t.Fatalf("convert: exit %d stderr %q", code, errOut)
	}
	if want := "[Event \"?\"]\n[Site \"?\"]\n[Date \"????.??.??\"]\n[Round \"?\"]\n[White \"?\"]\n[Black \"?\"]\n[Result \"*\"]\n\n1. d4 *\n\n"; out != want {
		t.Errorf("convert out = %q, want %q", out, want)
	}

	out, _, code = runCLI(t, stub, "convert", "--seven-tag-roster", "--quiet")
	if code != 0 || out != "" {
		t.Errorf("stub alone: exit %d out %q", code, out)
	}

	// A trailing game of nothing but stray tokens is not dealt with by
	// the C tool either (it stops at end of input): emitted nowhere, no
	// warning, exit 0.
	out, errOut, code = runCLI(t, real+strayStub, "convert", "--quiet")
	if code != 0 || strings.Contains(out, "stray") || errOut != "" {
		t.Errorf("trailing stray-only game: exit %d out %q stderr %q", code, out, errOut)
	}

	// The stub is not numbered: a broken game after it is still game 2.
	_, errOut, code = runCLI(t, real+"[Event \"two\"]\n\n1. e4 e4 *\n\n"+stub, "convert", "--quiet")
	if code != 2 || !strings.Contains(errOut, "game 2") || strings.Contains(errOut, "game 3") {
		t.Errorf("numbering: exit %d stderr %q", code, errOut)
	}
	_, errOut, _ = runCLI(t, stub+"{comment}\n\n"+"1. e4 e4 *\n", "convert", "--quiet")
	if !strings.Contains(errOut, "game 1") {
		t.Errorf("a stub followed by a comment is a zero-move game of its own, so the broken game is game 2: stderr %q", errOut)
	}

	for _, args := range [][]string{{"validate", "--summary"}, {"info"}, {"filter", "--quiet", "--result", "1-0"}} {
		out, _, code := runCLI(t, real+stub, args...)
		switch args[0] {
		case "validate":
			if code != 0 || !strings.Contains(out, "1 of 1 games valid") {
				t.Errorf("%v: exit %d out %q", args, code, out)
			}
		case "info":
			if code != 0 || !strings.Contains(out, "games:   1\n") {
				t.Errorf("%v: exit %d out %q", args, code, out)
			}
		case "filter":
			if code != 0 || out != "" {
				t.Errorf("%v: exit %d out %q", args, code, out)
			}
		}
	}
}
