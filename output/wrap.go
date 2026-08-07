package output

import (
	"bufio"
	"io"
)

// defaultLineWidth is used when Options.LineWidth is zero.
const defaultLineWidth = 75

// lineWriter accumulates whitespace-separated tokens and flushes them as
// lines no wider than width, breaking before the token that would overflow
// the current line. A token that is itself wider than width is placed
// alone on its own line.
type lineWriter struct {
	w       *bufio.Writer
	width   int
	line    []byte
	noSpace bool // suppress the separating space before the next token
	err     error
}

func newLineWriter(w io.Writer, width int) *lineWriter {
	if width <= 0 {
		width = defaultLineWidth
	}
	return &lineWriter{w: bufio.NewWriter(w), width: width}
}

// WriteToken appends a single token (no embedded spaces) to the current
// line, wrapping to a new line first if adding it (plus a separating
// space) would exceed the configured width.
func (lw *lineWriter) WriteToken(tok string) {
	if lw.err != nil || tok == "" {
		return
	}
	sep := 1
	if len(lw.line) == 0 || lw.noSpace {
		sep = 0
	}
	lw.noSpace = false
	if len(lw.line) > 0 && len(lw.line)+sep+len(tok) > lw.width {
		lw.flushLine()
		sep = 0
	}
	if sep == 1 {
		lw.line = append(lw.line, ' ')
	}
	lw.line = append(lw.line, tok...)
}

// SuppressSpace prevents the separating space before the next token
// written (used so "(" glues to the move/comment that follows it, and so
// a token immediately preceding ")" glues to it).
func (lw *lineWriter) SuppressSpace() {
	lw.noSpace = true
}

// flushLine writes the accumulated line (if any) followed by a newline.
func (lw *lineWriter) flushLine() {
	if lw.err != nil {
		return
	}
	if len(lw.line) == 0 {
		return
	}
	if _, err := lw.w.Write(lw.line); err != nil {
		lw.err = err
		return
	}
	if _, err := lw.w.Write([]byte("\n")); err != nil {
		lw.err = err
		return
	}
	lw.line = lw.line[:0]
}

// Close flushes any remaining partial line and the underlying buffered
// writer.
func (lw *lineWriter) Close() error {
	lw.flushLine()
	if lw.err != nil {
		return lw.err
	}
	return lw.w.Flush()
}
