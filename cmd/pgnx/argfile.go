package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// expandArgs replaces every argument of the form @FILE with the arguments
// read from that argument file (pgnx's form of pgn-extract's -A argsfile,
// in pgnx's own grammar; ADR 0004). The expansion runs before cobra sees
// the arguments, so a file may hold anything the command line may: the
// subcommand, flags, their values, input files and further @FILE
// arguments. A `--` ends expansion, on the command line or in a file:
// every argument after it is passed through verbatim, as cobra treats
// them as positional.
//
// The grammar of an argument file: one argument per line, except that a
// line starting with '-' is a flag followed by its value, split at the
// first run of white space (so `--tag White "Fischer"` is the two
// arguments `--tag` and `White "Fischer"`). Leading and trailing white
// space is trimmed; blank lines and comment lines, whose first non-blank
// character is '%', are ignored. Paths in a file, including nested @FILE
// lines, are relative to the working directory, not to the file.
func expandArgs(args []string) ([]string, error) {
	e := &argumentExpander{}
	if err := e.expand(args); err != nil {
		return nil, err
	}
	return e.out, nil
}

// argumentExpander accumulates the expanded arguments. stack holds the
// argument files being expanded, outermost first, so a file that includes
// itself (directly or through another file) is reported rather than
// recursed into forever.
type argumentExpander struct {
	out     []string
	stack   []string
	stopped bool // a `--` has been seen; no further expansion
}

// expand appends args to the output, expanding each @FILE argument in
// place until a `--` is seen.
func (e *argumentExpander) expand(args []string) error {
	for _, a := range args {
		switch {
		case e.stopped:
			e.out = append(e.out, a)
		case a == "--":
			e.stopped = true
			e.out = append(e.out, a)
		case len(a) > 1 && a[0] == '@':
			if err := e.expandFile(a[1:]); err != nil {
				return err
			}
		default:
			e.out = append(e.out, a)
		}
	}
	return nil
}

// expandFile reads one argument file and expands its arguments, with the
// file on the stack so that an include cycle is an error.
func (e *argumentExpander) expandFile(path string) error {
	for _, open := range e.stack {
		if open == path {
			return fmt.Errorf("argument file %s includes itself", path)
		}
	}
	args, err := readArgumentFile(path)
	if err != nil {
		return err
	}
	e.stack = append(e.stack, path)
	err = e.expand(args)
	e.stack = e.stack[:len(e.stack)-1]
	return err
}

// readArgumentFile parses one argument file into arguments, without
// expanding any nested @FILE lines.
func readArgumentFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("argument file: %w", err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '%' {
			continue
		}
		if line[0] == '-' {
			if i := strings.IndexAny(line, " \t"); i >= 0 {
				out = append(out, line[:i], strings.TrimSpace(line[i:]))
				continue
			}
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("argument file %s: %w", path, err)
	}
	return out, nil
}
