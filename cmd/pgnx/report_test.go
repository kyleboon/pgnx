package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/kyleboon/pgnx/pgn"
)

func gameItem(n, line int, skipped ...error) Item {
	return Item{Source: "in.pgn", Number: n, Game: &pgn.Game{StartLine: line}, Skipped: skipped}
}

// TestReportCountsCleanGames: a game with no errors and no skipped
// tokens succeeds, nothing is logged, and the run exits 0.
func TestReportCountsCleanGames(t *testing.T) {
	var log bytes.Buffer
	rep := newRunReport(&log, &log)
	rep.record(gameItem(1, 1), gameVerdict{})
	rep.record(gameItem(2, 5), gameVerdict{})
	if rep.total != 2 || rep.succeeded() != 2 {
		t.Errorf("total %d succeeded %d, want 2 2", rep.total, rep.succeeded())
	}
	if log.Len() != 0 {
		t.Errorf("log = %q, want empty", log.String())
	}
	if err := rep.exit(); err != nil {
		t.Errorf("exit = %v, want nil", err)
	}
}

// TestReportLogsEachErrorAndFailsTheGame: every error in the verdict is
// logged as "<src>:<line>: game <n>: <err>", the game counts as failed
// and the run exits 2.
func TestReportLogsEachErrorAndFailsTheGame(t *testing.T) {
	var log bytes.Buffer
	rep := newRunReport(&log, &log)
	rep.record(gameItem(1, 1), gameVerdict{})
	rep.record(gameItem(2, 7), gameVerdict{Errors: []error{errors.New("illegal move"), errors.New("moves dropped")}})
	want := "in.pgn:7: game 2: illegal move\nin.pgn:7: game 2: moves dropped\n"
	if log.String() != want {
		t.Errorf("log = %q, want %q", log.String(), want)
	}
	if rep.total != 2 || rep.succeeded() != 1 {
		t.Errorf("total %d succeeded %d, want 2 1", rep.total, rep.succeeded())
	}
	if !errors.Is(rep.exit(), errProblems) {
		t.Errorf("exit = %v, want errProblems", rep.exit())
	}
}

// TestReportSkippedTokensFailTheGame: a game with skipped tokens is
// logged as warnings (before any error) and never counts as succeeded,
// even with an empty verdict.
func TestReportSkippedTokensFailTheGame(t *testing.T) {
	var log bytes.Buffer
	rep := newRunReport(&log, &log)
	rep.record(gameItem(1, 3, errors.New("skipped token \"-\"")), gameVerdict{})
	rep.record(gameItem(2, 9, errors.New("skipped token \"-\"")), gameVerdict{Errors: []error{errors.New("bad")}})
	want := "in.pgn:3: game 1: warning: skipped token \"-\"\n" +
		"in.pgn:9: game 2: warning: skipped token \"-\"\n" +
		"in.pgn:9: game 2: bad\n"
	if log.String() != want {
		t.Errorf("log = %q, want %q", log.String(), want)
	}
	if rep.succeeded() != 0 {
		t.Errorf("succeeded = %d, want 0", rep.succeeded())
	}
	if !errors.Is(rep.exit(), errProblems) {
		t.Errorf("exit = %v, want errProblems", rep.exit())
	}
}

// TestReportStopAfterStillLogsTheFailingGame: Stop is a routing fact,
// not a verdict on the game; the game's errors are still logged and
// counted.
func TestReportStopAfterStillLogsTheFailingGame(t *testing.T) {
	var log bytes.Buffer
	rep := newRunReport(&log, &log)
	rep.record(gameItem(1, 1), gameVerdict{Stop: true})
	if rep.succeeded() != 1 || log.Len() != 0 {
		t.Errorf("clean stop: succeeded %d log %q", rep.succeeded(), log.String())
	}
	rep.record(gameItem(2, 4), gameVerdict{Errors: []error{errors.New("broken")}, Stop: true})
	if !strings.Contains(log.String(), "in.pgn:4: game 2: broken\n") {
		t.Errorf("failing stop game not logged: %q", log.String())
	}
	if rep.total != 2 || rep.succeeded() != 1 {
		t.Errorf("total %d succeeded %d, want 2 1", rep.total, rep.succeeded())
	}
}

// TestReportIOErrorFailsTheRun: a source's failure goes to the I/O
// writer as "<src>: <err>", is not a game, and fails the run even when
// every game succeeded. The two writers are separate so validate can
// discard findings under --summary without hiding I/O errors.
func TestReportIOErrorFailsTheRun(t *testing.T) {
	var findings, ioLog bytes.Buffer
	rep := newRunReport(&findings, &ioLog)
	rep.record(gameItem(1, 1), gameVerdict{})
	rep.record(Item{Source: "missing.pgn", Number: 1, Err: errors.New("no such file")}, gameVerdict{})
	if rep.total != 1 || rep.succeeded() != 1 {
		t.Errorf("total %d succeeded %d, want 1 1", rep.total, rep.succeeded())
	}
	if findings.Len() != 0 || ioLog.String() != "missing.pgn: no such file\n" {
		t.Errorf("findings %q ioLog %q", findings.String(), ioLog.String())
	}
	if !errors.Is(rep.exit(), errProblems) {
		t.Errorf("exit = %v, want errProblems", rep.exit())
	}
}

// TestReportIOOnlyIgnoresFailedGames: info's mode. Failed games are
// still logged, but only an I/O error fails the run.
func TestReportIOOnlyIgnoresFailedGames(t *testing.T) {
	var log bytes.Buffer
	rep := newRunReport(&log, &log)
	rep.ioOnly = true
	rep.record(gameItem(1, 2, errors.New("skipped token")), gameVerdict{})
	if !strings.Contains(log.String(), "warning: skipped token") {
		t.Errorf("warning not logged: %q", log.String())
	}
	if err := rep.exit(); err != nil {
		t.Errorf("exit = %v, want nil", err)
	}
	rep.record(Item{Source: "missing.pgn", Err: errors.New("no such file")}, gameVerdict{})
	if !errors.Is(rep.exit(), errProblems) {
		t.Errorf("exit after I/O error = %v, want errProblems", rep.exit())
	}
}

// TestReportRunFailure: a failure of the run itself (a destination that
// could not be flushed at close) is logged to the I/O writer and fails
// the run.
func TestReportRunFailure(t *testing.T) {
	var log bytes.Buffer
	rep := newRunReport(&log, &log)
	rep.record(gameItem(1, 1), gameVerdict{})
	rep.fail(errors.New("out.pgn: write: no space left on device"))
	if log.String() != "out.pgn: write: no space left on device\n" {
		t.Errorf("log = %q", log.String())
	}
	if !errors.Is(rep.exit(), errProblems) {
		t.Errorf("exit = %v, want errProblems", rep.exit())
	}
}

// TestReportSummaryLines: the pgn-extract summary line (main.c) with the
// count the command supplies, and validate's tally. quiet suppresses the
// summary line.
func TestReportSummaryLines(t *testing.T) {
	var log bytes.Buffer
	rep := newRunReport(&log, &log)
	rep.record(gameItem(1, 1), gameVerdict{})
	rep.record(gameItem(2, 3), gameVerdict{Errors: []error{errors.New("bad")}})
	log.Reset()

	var out bytes.Buffer
	rep.summary(&out, rep.succeeded())
	if out.String() != "1 game matched out of 2.\n" {
		t.Errorf("summary = %q", out.String())
	}
	out.Reset()
	rep.summary(&out, 0)
	if out.String() != "0 games matched out of 2.\n" {
		t.Errorf("summary = %q", out.String())
	}
	out.Reset()
	rep.quiet = true
	rep.summary(&out, 1)
	if out.Len() != 0 {
		t.Errorf("quiet summary = %q, want nothing", out.String())
	}
	rep.validSummary(&out)
	if out.String() != "1 of 2 games valid\n" {
		t.Errorf("valid summary = %q", out.String())
	}
}

// TestReportEachStopsWhenAsked: each drives fn over the iterator's
// games, records every verdict, passes Err items straight to the
// report, and stops reading once a verdict says so.
func TestReportEachStopsWhenAsked(t *testing.T) {
	items := []Item{
		gameItem(1, 1),
		{Source: "missing.pgn", Number: 1, Err: errors.New("no such file")},
		gameItem(2, 4),
		gameItem(3, 8),
	}
	yielded := 0
	seq := func(yield func(Item) bool) {
		for _, it := range items {
			yielded++
			if !yield(it) {
				return
			}
		}
	}
	var log bytes.Buffer
	rep := newRunReport(&log, &log)
	var seen []int
	rep.each(seq, func(n int, g *pgn.Game) gameVerdict {
		seen = append(seen, n)
		return gameVerdict{Stop: n == 2}
	})
	if len(seen) != 2 || seen[0] != 1 || seen[1] != 2 {
		t.Errorf("fn saw games %v, want [1 2]", seen)
	}
	if yielded != 3 {
		t.Errorf("iterator yielded %d items, want 3 (stopped after game 2)", yielded)
	}
	if rep.total != 2 || !rep.ioFailed || log.String() != "missing.pgn: no such file\n" {
		t.Errorf("total %d ioFailed %v log %q", rep.total, rep.ioFailed, log.String())
	}
}
