package agent

import (
	"fmt"
	"sync"
	"time"

	"github.com/martianzhang/aigc-cli/internal/types"
)

const (
	// DefaultShellTimeout is the fallback used when neither the model nor the
	// user configured a timeout. It is generous on purpose: builds and test
	// suites are the most common long-running bash calls.
	DefaultShellTimeout = time.Duration(types.DefaultBashTimeoutSeconds) * time.Second
	// MaxShellTimeout bounds a model-supplied timeout so a typo (or a unit
	// mistake, e.g. milliseconds sent as seconds) cannot pin a turn for days.
	MaxShellTimeout = 24 * time.Hour
)

// shellTimeoutConfig holds the user-configured default for bash commands.
//
// The agent package cannot read config.yaml itself (that would invert the
// dependency on the CLI layer), so cmd/root.go resolves
// defaults.chat.bash.timeout once at startup through SetShellTimeout. Tests and
// library callers keep the built-in default.
var shellTimeoutConfig struct {
	mu      sync.RWMutex
	timeout time.Duration
}

// SetShellTimeout configures the default timeout for bash commands. d <= 0
// restores the built-in default.
func SetShellTimeout(d time.Duration) {
	shellTimeoutConfig.mu.Lock()
	shellTimeoutConfig.timeout = d
	shellTimeoutConfig.mu.Unlock()
}

// shellDefaultTimeout returns the configured default, or DefaultShellTimeout.
func shellDefaultTimeout() time.Duration {
	shellTimeoutConfig.mu.RLock()
	d := shellTimeoutConfig.timeout
	shellTimeoutConfig.mu.RUnlock()
	if d > 0 {
		return d
	}
	return DefaultShellTimeout
}

// resolveShellTimeout picks the per-call timeout when the model supplied one,
// otherwise the configured default.
func resolveShellTimeout(requested time.Duration) time.Duration {
	if requested > 0 {
		return requested
	}
	return shellDefaultTimeout()
}

// parseShellTimeout converts the model-facing `timeout` argument (seconds) into
// a duration. Zero means "not supplied"; negative and absurd values are
// rejected so the model gets an actionable error instead of a silent clamp.
func parseShellTimeout(seconds float64) (time.Duration, error) {
	if seconds == 0 {
		return 0, nil
	}
	if seconds < 0 {
		return 0, fmt.Errorf("timeout must be a positive number of seconds")
	}
	d := time.Duration(seconds * float64(time.Second))
	if d > MaxShellTimeout {
		return 0, fmt.Errorf("timeout must be at most %d seconds", int(MaxShellTimeout.Seconds()))
	}
	return d, nil
}
