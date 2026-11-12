// Package ui centralizes process output. CLI code keeps writing to stdout,
// while MCP swaps Out to stderr so tool progress cannot corrupt the JSON-RPC
// stream. Library code must use these helpers instead of fmt.Print*.
package ui

import (
	"fmt"
	"io"
	"os"
)

// Nil overrides mean "use the live os.Stdout/os.Stderr". Reading the process
// files dynamically keeps tests that redirect os.Stdout (and callers that swap
// the process streams) working.
var (
	outOverride io.Writer
	errOverride io.Writer
)

// Out returns the current stdout writer.
func Out() io.Writer {
	if outOverride != nil {
		return outOverride
	}
	return os.Stdout
}

// Err returns the current stderr writer.
func Err() io.Writer {
	if errOverride != nil {
		return errOverride
	}
	return os.Stderr
}

// SetOut overrides stdout and returns a restore function.
func SetOut(w io.Writer) func() {
	old := outOverride
	outOverride = w
	return func() { outOverride = old }
}

// SetErr overrides stderr and returns a restore function.
func SetErr(w io.Writer) func() {
	old := errOverride
	errOverride = w
	return func() { errOverride = old }
}

// Printf writes formatted text to Out.
func Printf(format string, args ...any) { _, _ = fmt.Fprintf(Out(), format, args...) }

// Println writes arguments to Out followed by a newline.
func Println(args ...any) { _, _ = fmt.Fprintln(Out(), args...) }

// Print writes arguments to Out.
func Print(args ...any) { _, _ = fmt.Fprint(Out(), args...) }

// Fprintf writes formatted text to w.
func Fprintf(w io.Writer, format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }

// Fprintln writes arguments to w followed by a newline.
func Fprintln(w io.Writer, args ...any) { _, _ = fmt.Fprintln(w, args...) }

// Fprint writes arguments to w.
func Fprint(w io.Writer, args ...any) { _, _ = fmt.Fprint(w, args...) }

// Eprintf writes formatted text to Err.
func Eprintf(format string, args ...any) { _, _ = fmt.Fprintf(Err(), format, args...) }

// Eprintln writes arguments to Err followed by a newline.
func Eprintln(args ...any) { _, _ = fmt.Fprintln(Err(), args...) }

// Eprint writes arguments to Err.
func Eprint(args ...any) { _, _ = fmt.Fprint(Err(), args...) }
