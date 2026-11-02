package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/kyleboon/pgnx/output"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// errDiskFull stands in for the write error a full disk produces.
var errDiskFull = errors.New("disk full")

// failingWriter accepts nothing: every Write fails with errDiskFull.
type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) { return 0, errDiskFull }

// TestDestinationBuffersAndReportsWriteErrorAtClose pins the destination
// type's contract: games are buffered for the whole run, so a write the
// sink refuses does not fail the game, and Close flushes the buffer and
// reports the failure, which is what makes a full disk exit 2.
func TestDestinationBuffersAndReportsWriteErrorAtClose(t *testing.T) {
	d := newBufferedWriter(failingWriter{}, nil, output.Options{Format: output.SAN})
	s := pgn.NewScanner(strings.NewReader(fools), pgn.ParserOptions{})
	if !s.Next() {
		t.Fatal(s.Err())
	}
	g := s.Game()
	r, err := replay.Game(g, replay.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeGameTo(d, g, r); err != nil {
		t.Fatalf("a buffered write must not reach the sink: %v", err)
	}
	if err := d.Close(); !errors.Is(err, errDiskFull) {
		t.Errorf("Close: got %v, want %v", err, errDiskFull)
	}
}

// TestConvertExitsTwoWhenOutputCannotBeWritten pins the exit status end
// to end: stdout refuses every write, every game was otherwise fine, and
// the run still exits 2 with the failure on the log.
func TestConvertExitsTwoWhenOutputCannotBeWritten(t *testing.T) {
	root := NewRootCmd()
	var errb bytes.Buffer
	root.SetOut(failingWriter{})
	root.SetErr(&errb)
	root.SetIn(strings.NewReader(fools))
	root.SetArgs([]string{"convert", "--quiet"})
	if code := execute(root); code != 2 {
		t.Errorf("exit %d, want 2; stderr %q", code, errb.String())
	}
	if !strings.Contains(errb.String(), errDiskFull.Error()) {
		t.Errorf("stderr %q should report the write error", errb.String())
	}
}

// TestFilterUnmatchedToStdoutKeepsGameOrder pins that --output and
// --unmatched both on stdout interleave matched and unmatched games in
// input order, as one buffer per sink requires when two destinations
// share it.
func TestFilterUnmatchedToStdoutKeepsGameOrder(t *testing.T) {
	in := "[White \"Fischer\"]\n\n1. e4 *\n\n[White \"Tal\"]\n\n1. d4 *\n\n[White \"Fischer\"]\n\n1. c4 *\n"
	out, _, code := runCLI(t, in, "filter", "--quiet", "--player", "Fischer", "--unmatched", "-")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	e4, d4, c4 := strings.Index(out, "1. e4"), strings.Index(out, "1. d4"), strings.Index(out, "1. c4")
	if e4 < 0 || d4 < 0 || c4 < 0 || !(e4 < d4 && d4 < c4) {
		t.Errorf("games out of input order:\n%s", out)
	}
}
