package main

import (
	"bytes"
	"io"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kyleboon/pgnx/output"
	"github.com/kyleboon/pgnx/pgn"
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

// collect drains the iterator into a slice.
func collect(t *testing.T, seq iter.Seq[Item]) []Item {
	t.Helper()
	var items []Item
	for it := range seq {
		items = append(items, it)
	}
	return items
}

func writeTemp(t *testing.T, name, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestGamesNumbersAcrossFiles: the iterator walks the files in order,
// labels each game with its file and numbers games across all files.
func TestGamesNumbersAcrossFiles(t *testing.T) {
	a := writeTemp(t, "a.pgn", "[Event \"one\"]\n\n1. e4 *\n\n[Event \"two\"]\n\n1. d4 *\n")
	b := writeTemp(t, "b.pgn", "[Event \"three\"]\n\n1. c4 *\n")
	items := collect(t, games([]string{a, b}, strings.NewReader(""), pgn.ParserOptions{}))
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3: %+v", len(items), items)
	}
	for i, want := range []struct {
		src, event string
	}{{a, "one"}, {a, "two"}, {b, "three"}} {
		it := items[i]
		event, _ := it.Game.Tag("Event")
		if it.Err != nil || it.Source != want.src || it.Number != i+1 || event != want.event {
			t.Errorf("item %d = {Source %q Number %d Event %q Err %v}, want {%q %d %q nil}", i, it.Source, it.Number, event, it.Err, want.src, i+1, want.event)
		}
	}
}

// TestGamesReadsStdinWhenNoFiles: with no files the iterator reads the
// given reader, labelled "stdin".
func TestGamesReadsStdinWhenNoFiles(t *testing.T) {
	items := collect(t, games(nil, strings.NewReader("1. e4 *\n"), pgn.ParserOptions{}))
	if len(items) != 1 || items[0].Source != "stdin" || items[0].Number != 1 || items[0].Game == nil {
		t.Fatalf("items = %+v", items)
	}
}

// TestGamesPassesParserOptions: the options given are the ones the
// parser runs with (nested comments here), with no flag lookup inside.
func TestGamesPassesParserOptions(t *testing.T) {
	text := "1. e4 {outer {inner} still} e5 *\n"
	plain := collect(t, games(nil, strings.NewReader(text), pgn.ParserOptions{}))
	nested := collect(t, games(nil, strings.NewReader(text), pgn.ParserOptions{NestedComments: true}))
	if len(plain) != 1 || len(nested) != 1 {
		t.Fatalf("plain %d nested %d items", len(plain), len(nested))
	}
	if got := plain[0].Game.Moves[0].Comments(); len(got) != 1 || got[0] != "outer {inner" {
		t.Errorf("plain comment = %q", got)
	}
	if got := nested[0].Game.Moves[0].Comments(); len(got) != 1 || got[0] != "outer {inner} still" {
		t.Errorf("nested comment = %q", got)
	}
}

// TestGamesSkippedTokens: the tokens the parser discarded ride on the
// item, so the report can count the game as failed.
func TestGamesSkippedTokens(t *testing.T) {
	items := collect(t, games(nil, strings.NewReader("1. e4 - e5 *\n"), pgn.ParserOptions{}))
	if len(items) != 1 || len(items[0].Skipped) != 1 {
		t.Fatalf("items = %+v", items)
	}
}

// TestGamesReportsUnreadableFile: a file that cannot be opened yields one
// Err item for it and the walk goes on with the next file.
func TestGamesReportsUnreadableFile(t *testing.T) {
	b := writeTemp(t, "b.pgn", "1. c4 *\n")
	missing := filepath.Join(t.TempDir(), "missing.pgn")
	items := collect(t, games([]string{missing, b}, strings.NewReader(""), pgn.ParserOptions{}))
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2: %+v", len(items), items)
	}
	if items[0].Source != missing || items[0].Err == nil || items[0].Game != nil {
		t.Errorf("item 0 = %+v, want an Err item for %s", items[0], missing)
	}
	if items[1].Source != b || items[1].Err != nil || items[1].Number != 1 {
		t.Errorf("item 1 = %+v, want game 1 from %s", items[1], b)
	}
}

// TestGamesReportsFatalScanError: a fatal parse error ends the source
// with an Err item, after the games scanned before it.
func TestGamesReportsFatalScanError(t *testing.T) {
	items := collect(t, games(nil, strings.NewReader("1. e4 *\n\n1. d4 {unterminated\n"), pgn.ParserOptions{}))
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2: %+v", len(items), items)
	}
	if items[0].Err != nil || items[0].Number != 1 {
		t.Errorf("item 0 = %+v, want game 1", items[0])
	}
	if items[1].Err == nil || !strings.Contains(items[1].Err.Error(), "unterminated") || items[1].Source != "stdin" {
		t.Errorf("item 1 = %+v, want the fatal scan error", items[1])
	}
}

// TestGamesDoesNotReadAheadOfCurrentGame is ADR 0003's streaming guard:
// the iterator yields a game as soon as its text is complete and reads
// no further until the consumer asks for the next one. The input is an
// io.Pipe whose writer releases game 2's bytes only after game 1 has
// been yielded and rendered in full, so an iterator that read ahead
// would block forever on the pipe (and the test would time out).
func TestGamesDoesNotReadAheadOfCurrentGame(t *testing.T) {
	pr, pw := io.Pipe()
	released := make(chan struct{})
	go func() {
		defer pw.Close()
		io.WriteString(pw, "[Event \"one\"]\n\n1. e4 e5 1-0\n")
		<-released
		io.WriteString(pw, "[Event \"two\"]\n\n1. d4 d5 0-1\n")
	}()

	var emitted bytes.Buffer
	w := output.NewWriter(&emitted, output.Options{Format: output.Source, LineWidth: 75})
	done := make(chan []string)
	go func() {
		var events []string
		for it := range games(nil, pr, pgn.ParserOptions{SkipTagsOnly: true}) {
			if it.Err != nil {
				t.Error(it.Err)
				break
			}
			if err := w.WriteGame(it.Game, nil); err != nil {
				t.Error(err)
			}
			event, _ := it.Game.Tag("Event")
			events = append(events, event)
			if it.Number == 1 {
				if !strings.Contains(emitted.String(), "1. e4 e5 1-0") {
					t.Errorf("game 1 not fully emitted before game 2 is released: %q", emitted.String())
				}
				close(released)
			}
		}
		done <- events
	}()

	select {
	case events := <-done:
		if strings.Join(events, ",") != "one,two" {
			t.Errorf("events = %v, want [one two]", events)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("iterator blocked reading ahead of game 1 (game 2's bytes were never released)")
	}
}
