// Command pgnx is a Go port of pgn-extract: a command-line tool for
// manipulating collections of chess games recorded in PGN.
package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// errProblems is a sentinel error returned by subcommands to signal that
// processing completed but one or more games failed (a bad game, or an
// unreadable input file). exitCode maps it to exit status 2, matching the
// pgn-extract C oracle's behavior of continuing past bad games but
// reporting failure at the end.
var errProblems = errors.New("pgnx: one or more games failed")

// version is pgnx's release version, reported by "pgnx --version".
const version = "0.1.0-dev"

// exitCode maps an error returned by (*cobra.Command).Execute to a process
// exit status: 0 for no error (handled by the caller before calling
// exitCode), 2 for errProblems, and 1 for anything else (usage errors,
// flag parsing errors, or other command failures).
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, errProblems) {
		return 2
	}
	return 1
}

// execute runs root and maps its error to a process exit status. Because
// the command tree sets SilenceErrors, cobra never prints errors itself;
// execute prints any non-sentinel error to the command's error stream with
// a "pgnx: " prefix. The errProblems sentinel stays silent: the per-game
// error lines and the summary have already been written to the log writer,
// so printing the sentinel too would only duplicate that report.
func execute(root *cobra.Command) int {
	err := root.Execute()
	if err != nil && !errors.Is(err, errProblems) {
		fmt.Fprintf(root.ErrOrStderr(), "pgnx: %v\n", err)
	}
	return exitCode(err)
}

// NewRootCmd builds the pgnx root command and its subcommands. It is a
// factory (rather than a package-level singleton) so tests can construct
// an isolated command tree with its own in/out/err streams and flag state.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "pgnx",
		Short:         "pgnx manipulates collections of chess games in PGN format",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().BoolP("quiet", "q", false, "suppress the summary line")
	root.PersistentFlags().String("log", "", "write log/summary output to FILE instead of stderr")

	root.AddCommand(newConvertCmd())
	root.AddCommand(newFilterCmd())
	root.AddCommand(newValidateCmd())
	root.AddCommand(newInfoCmd())

	return root
}

// logWriter returns the writer that per-game error messages and the
// summary line should be sent to: the file named by --log, if set,
// otherwise cmd.ErrOrStderr(). The caller is responsible for closing the
// returned io.Closer (which is nil when no file was opened).
func logWriter(cmd *cobra.Command) (io.Writer, io.Closer, error) {
	logPath, err := cmd.Flags().GetString("log")
	if err != nil {
		return nil, nil, err
	}
	if logPath == "" {
		return cmd.ErrOrStderr(), nil, nil
	}
	f, err := openLogFile(logPath)
	if err != nil {
		return nil, nil, err
	}
	return f, f, nil
}
