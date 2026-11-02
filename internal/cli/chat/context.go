package chat

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/cli/options"
)

// agentsFileName is the project/global context file, compatible with the
// AGENTS.md convention used by many coding agents.
const agentsFileName = "AGENTS.md"

// loadAgentsContext returns the concatenated AGENTS.md context (global config-dir
// file, then project files from outermost to innermost), or "" when none exist.
func loadAgentsContext() string {
	globalPath := ""
	if dir := options.ConfigDir(); dir != "" {
		globalPath = filepath.Join(dir, agentsFileName)
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	return agentsContextFrom(cwd, globalPath)
}

// runtimeContext returns the guidance appended to the system prompt: the
// platform/shell notes (only on platforms where it matters) followed by the
// AGENTS.md project context.
func runtimeContext() string {
	var parts []string
	if p := agent.PlatformPrompt(); p != "" {
		parts = append(parts, p)
	}
	if ctx := loadAgentsContext(); ctx != "" {
		parts = append(parts, ctx)
	}
	return strings.Join(parts, "\n\n")
}

// agentsContextFrom builds the context block from a global file and the
// AGENTS.md files found walking up from startDir. Order is global first, then
// project files outermost→innermost (most specific last). The upward walk stops
// at the repository root (the first ancestor containing .git) or the filesystem
// root, whichever comes first.
func agentsContextFrom(startDir, globalPath string) string {
	var parts []string

	if globalPath != "" {
		if s := readAgentsFile(globalPath); s != "" {
			parts = append(parts, s)
		}
	}

	// Collect project files walking up; innermost-first, then reversed.
	var chain []string
	dir := startDir
	for dir != "" {
		if s := readAgentsFile(filepath.Join(dir, agentsFileName)); s != "" {
			chain = append(chain, s)
		}
		if isRepoRoot(dir) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	for _, p := range slices.Backward(chain) {
		parts = append(parts, p)
	}

	if len(parts) == 0 {
		return ""
	}
	return "# Project context (AGENTS.md)\n\n" + strings.Join(parts, "\n\n---\n\n")
}

func readAgentsFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// isRepoRoot reports whether dir contains a .git entry (file or directory).
func isRepoRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}
