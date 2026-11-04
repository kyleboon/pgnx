package main

import (
	"bufio"
	"io"
	"os"

	"github.com/kyleboon/pgnx/output"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// destination is where a command sends emitted games (GLOSSARY.md): it
// supplies the output.Writer a game is rendered with (one fixed writer,
// or one per ECO file under --eco-split). Every writer renders into one
// bufio.Writer per file (or stdout) that lives for the whole run (ADR
// 0003: the output package writes a line at a time, so an unbuffered
// file would cost a syscall per tag line). Close flushes the buffers and
// closes the files, returning the first error, so an output that could
// not be written in full (a full disk) still fails the run.
type destination interface {
	writerFor(g *pgn.Game) (*output.Writer, error)
	io.Closer
}

// bufferedDestination is one file (or stdout) with its run-wide buffer
// and the one output.Writer that renders into it.
type bufferedDestination struct {
	w   *output.Writer
	buf *bufio.Writer
	// file is closed after the final flush; nil for stdout.
	file io.Closer
}

// newBufferedDestination buffers dst for the run and closes file, when
// not nil, at Close.
func newBufferedDestination(dst io.Writer, file io.Closer, opts output.Options) *bufferedDestination {
	buf := bufio.NewWriter(dst)
	return &bufferedDestination{w: output.NewWriter(buf, opts), buf: buf, file: file}
}

func (b *bufferedDestination) writerFor(*pgn.Game) (*output.Writer, error) { return b.w, nil }

// openFileDestination creates (or, with mode os.O_APPEND, extends) the
// file name in the current directory as a buffered destination; mode is
// os.O_TRUNC or os.O_APPEND. The split and ECO-split destinations open
// their files through it.
func openFileDestination(name string, mode int, opts output.Options) (*bufferedDestination, error) {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|mode, 0o644)
	if err != nil {
		return nil, err
	}
	return newBufferedDestination(f, f, opts), nil
}

// Close ends the writer's output (the closing "]" of JSON output; nothing
// otherwise), flushes what the run buffered and closes the file. A flush
// error is the write error the file refused with; it wins over a close
// error.
func (b *bufferedDestination) Close() error {
	err := b.w.Close()
	if ferr := b.buf.Flush(); err == nil {
		err = ferr
	}
	if b.file != nil {
		if cerr := b.file.Close(); err == nil {
			err = cerr
		}
	}
	return err
}

// writeGameTo resolves the writer dest uses for g and writes g with its
// replay r: nil for a broken game, which the writer then renders in
// Source format, the only format that needs no replay.
func writeGameTo(dest destination, g *pgn.Game, r *replay.Result) error {
	w, err := dest.writerFor(g)
	if err != nil {
		return err
	}
	return w.WriteGame(g, r)
}
