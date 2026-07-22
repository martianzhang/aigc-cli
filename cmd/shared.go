package cmd

import (
	"io"
	"path"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// SharedConfig holds all shared configuration values that were previously
// individual global variables. Initialized in PersistentPreRunE.
type SharedConfig struct {
	CfgFile     string
	APIKey      string
	APIBase     string
	HTTPProxy   string
	Model       string
	JSONInput   string
	OutputDir   string
	Verbose     bool
	SavePrompt  bool
	Mode        string
	PrintConfig bool
	TimeoutFlag int
	Cfg         *types.Config // full parsed config (may be nil)
}

// SetSharedForTest sets the global shared config to a test-specific value and
// returns a cleanup function that restores the previous state. Tests should
// defer the cleanup:
//
//	defer SetSharedForTest(&SharedConfig{APIKey: "test", ...})()
func SetSharedForTest(sc *SharedConfig) func() {
	old := *shared
	*shared = *sc
	return func() { *shared = old }
}

// SetChatOutputForTest overrides the chat REPL output writers for testing and
// returns a cleanup function. Useful for capturing chat output in tests:
//
//	var stdout, stderr strings.Builder
//	defer SetChatOutputForTest(&stdout, &stderr)()
//
// matchAny returns true if name matches any of the glob patterns.
func matchAny(name string, patterns []string) bool {
	for _, p := range patterns {
		if matched, _ := path.Match(p, name); matched {
			return true
		}
	}
	return false
}

// isToolAllowed checks if a tool is allowed by global tools_enable/tools_disable rules.
// Empty enable list = all tools allowed. When enable is set, tool must match at least one pattern.
// disable is a blacklist applied on top.
func isToolAllowed(toolName string, enable, disable []string) bool {
	if len(enable) > 0 && !matchAny(toolName, enable) {
		return false
	}
	if matchAny(toolName, disable) {
		return false
	}
	return true
}

func SetChatOutputForTest(stdout, stderr io.Writer) func() {
	oldStdout, oldStderr := chatStdout, chatStderr
	chatStdout, chatStderr = stdout, stderr
	return func() { chatStdout, chatStderr = oldStdout, oldStderr }
}
