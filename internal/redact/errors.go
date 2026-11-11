package redact

import "fmt"

// ResidualError is returned by `redact check` when original values remain in
// the output. It carries exit code 2 so scripts can distinguish "found" from a
// runtime failure.
type ResidualError struct {
	Count int
}

func (e *ResidualError) Error() string {
	return fmt.Sprintf("residual sensitive data: %d occurrence(s)", e.Count)
}

// ExitCode implements the CLI's exit-code protocol.
func (e *ResidualError) ExitCode() int { return 2 }
