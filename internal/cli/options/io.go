package options

import (
	"io"
	"os"
)

var stdout io.Writer = os.Stdout
var stderr io.Writer = os.Stderr

// Stdout returns the current chat/REPL stdout writer.
func Stdout() io.Writer { return stdout }

// Stderr returns the current chat/REPL stderr writer.
func Stderr() io.Writer { return stderr }

// SetStdout overrides the stdout writer and returns a restore function.
func SetStdout(w io.Writer) func() {
	old := stdout
	stdout = w
	return func() { stdout = old }
}

// SetStderr overrides the stderr writer and returns a restore function.
func SetStderr(w io.Writer) func() {
	old := stderr
	stderr = w
	return func() { stderr = old }
}
