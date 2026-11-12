package options

import (
	"io"

	"github.com/martianzhang/aigc-cli/internal/ui"
)

// Stdout returns the current chat/REPL stdout writer.
func Stdout() io.Writer { return ui.Out() }

// Stderr returns the current chat/REPL stderr writer.
func Stderr() io.Writer { return ui.Err() }

// SetStdout overrides the stdout writer and returns a restore function.
func SetStdout(w io.Writer) func() { return ui.SetOut(w) }

// SetStderr overrides the stderr writer and returns a restore function.
func SetStderr(w io.Writer) func() { return ui.SetErr(w) }
