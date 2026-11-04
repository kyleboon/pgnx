package main

import (
	"strings"
	"testing"
)

// Every command normalises a parsed game once, before replay
// (pipeline.Normalise); a game the step rejects is emitted nowhere,
// counted as a failure, and makes the exit status 2.

const setUpWithoutFEN = "[Event \"a\"]\n[SetUp \"1\"]\n\n1. e4 *\n\n"

func TestFilterRejectsSetUpWithoutFEN(t *testing.T) {
	for _, extra := range [][]string{nil, {"--keep-broken"}} {
		args := append([]string{"filter", "--quiet", "--unmatched", "-", "--player", "Nobody"}, extra...)
		out, errOut, code := runCLI(t, setUpWithoutFEN+fools, args...)
		if code != 2 {
			t.Fatalf("%v: exit = %d, want 2", extra, code)
		}
		if strings.Contains(out, "1. e4 *") || !strings.Contains(out, "Qh4#") {
			t.Errorf("%v: rejected game emitted nowhere, unmatched good game kept; out=%q", extra, out)
		}
		if !strings.Contains(errOut, "game 1") || !strings.Contains(errOut, "SetUp") {
			t.Errorf("%v: stderr should name the game and the tag: %q", extra, errOut)
		}
	}
}

func TestValidateRejectsSetUpWithoutFEN(t *testing.T) {
	out, _, code := runCLI(t, setUpWithoutFEN+fools, "validate")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(out, "game 1") || !strings.Contains(out, "SetUp") || strings.Contains(out, "game 2") {
		t.Errorf("findings: %q", out)
	}
	out, _, code = runCLI(t, setUpWithoutFEN+fools, "validate", "--summary")
	if code != 2 || !strings.Contains(out, "1 of 2 games valid") {
		t.Errorf("summary: exit %d out %q", code, out)
	}
}

// Oracle for both inputs (apply.c play_moves replaces a non-PGN Result
// with "*" after check_result has filled a "?" from the token):
//
//	printf '[Result "?"]\n\n1. e4 *\n' | pgn-extract -Wepd --quiet
//	printf '[Result "1:0"]\n\n1. e4 1-0\n' | pgn-extract -Wepd --quiet
//	  -> ... c0 -; c1 *;   (both lines, both inputs)
func TestConvertEPDResultNormalised(t *testing.T) {
	for _, in := range []string{"[Result \"?\"]\n\n1. e4 *\n", "[Result \"1:0\"]\n\n1. e4 1-0\n"} {
		out, _, code := runCLI(t, in, "convert", "--quiet", "-f", "epd")
		if code != 0 {
			t.Fatalf("%q: exit = %d", in, code)
		}
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if len(lines) != 2 {
			t.Fatalf("%q: want 2 EPD lines, got %q", in, out)
		}
		for _, l := range lines {
			if !strings.HasSuffix(l, "c0 -; c1 *;") {
				t.Errorf("%q: line %q should end with c1 *;", in, l)
			}
		}
	}
}

// Oracle: printf '[Event "x"]\n[Result "1/2"]\n\n*\n' | pgn-extract -R<roster> --quiet
// with a roster file naming Event and Result -> [Result "1/2-1/2"].
func TestConvertRosterResultNormalised(t *testing.T) {
	out, _, code := runCLI(t, "[Event \"x\"]\n[Result \"1/2\"]\n\n*\n", "convert", "--quiet", "--roster", "Event,Result")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if want := "[Event \"x\"]\n[Result \"1/2-1/2\"]\n\n1/2-1/2\n\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

// The C parser normalises before any criterion sees the tag.
func TestFilterResultCriterionSeesNormalisedTag(t *testing.T) {
	in := "[Result \"1/2\"]\n\n1. e4 1/2-1/2\n\n[Result \"?\"]\n\n1. d4 1/2-1/2\n\n" + fools
	out, _, code := runCLI(t, in, "filter", "--quiet", "--result", "1/2-1/2")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "1. e4") || !strings.Contains(out, "1. d4") || strings.Contains(out, "Qh4#") {
		t.Errorf("out = %q", out)
	}
}

// TestConvertAddsChess960Variant is pinned against pgn-extract --quiet on
// the same input: a FEN game whose start is a Chess960 setup gets
// [Variant "chess 960"], emitted in the oracle's tag order (after ECO,
// before SetUp and FEN); the castling field it carries is unchanged.
// With -7 the Variant, SetUp and FEN tags follow the roster.
func TestConvertAddsChess960Variant(t *testing.T) {
	in := "[Event \"960\"]\n[ECO \"A00\"]\n[Result \"*\"]\n[Custom \"x\"]\n[FEN \"nqbrkbrn/pppppppp/8/8/8/8/PPPPPPPP/NQBRKBRN b KQkq - 0 1\"]\n\n1... e5 *\n"
	out, errOut, code := runCLI(t, in, "convert", "--quiet")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	want := `[Event "960"]
[Site "?"]
[Date "????.??.??"]
[Round "?"]
[White "?"]
[Black "?"]
[Result "*"]
[ECO "A00"]
[Variant "chess 960"]
[SetUp "1"]
[FEN "nqbrkbrn/pppppppp/8/8/8/8/PPPPPPPP/NQBRKBRN b KQkq - 0 1"]
[Custom "x"]

1... e5 *

`
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	out, _, _ = runCLI(t, in, "convert", "--quiet", "--seven-tag-roster")
	want = `[Event "960"]
[Site "?"]
[Date "????.??.??"]
[Round "?"]
[White "?"]
[Black "?"]
[Result "*"]
[Variant "chess 960"]
[SetUp "1"]
[FEN "nqbrkbrn/pppppppp/8/8/8/8/PPPPPPPP/NQBRKBRN b KQkq - 0 1"]

1... e5 *

`
	if out != want {
		t.Errorf("-7 got:\n%s\nwant:\n%s", out, want)
	}
}
