package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbletea"
)

// handleTabCompletion completes commands (/xxx) or shell commands (!xxx).
func (m *chatModel) handleTabCompletion() (tea.Model, tea.Cmd) {
	if m.state != tuiIdle {
		return m, nil
	}

	input := m.input.Value()
	if input == "" {
		return m, nil
	}

	// Shell command completion: !<cmd>
	if strings.HasPrefix(input, "!") {
		return m.completeShellCmd(input)
	}

	// Command completion: /<cmd>
	return m.completeCommand(input)
}

// completeCommand completes /-prefixed commands.
func (m *chatModel) completeCommand(input string) (tea.Model, tea.Cmd) {
	// Reset cycle if input changed since last Tab
	if m.cycleIdx < 0 || len(m.cycleMatches) == 0 || !strings.HasPrefix(strings.ToLower(m.cycleMatches[m.cycleIdx]), strings.ToLower(input)) {
		m.cycleMatches = nil
		for _, c := range m.completions {
			if strings.HasPrefix(strings.ToLower(c), strings.ToLower(input)) {
				m.cycleMatches = append(m.cycleMatches, c)
			}
		}
		m.cycleIdx = -1
	}

	if len(m.cycleMatches) == 0 {
		return m, nil
	}

	m.cycleIdx = (m.cycleIdx + 1) % len(m.cycleMatches)
	m.input.SetValue(m.cycleMatches[m.cycleIdx])
	m.input.CursorEnd()
	return m, nil
}

// completeShellCmd completes executables in PATH for !-prefixed input.
func (m *chatModel) completeShellCmd(input string) (tea.Model, tea.Cmd) {
	prefix := strings.TrimPrefix(input, "!")

	// Reset cycle if input changed since last Tab
	if m.cycleIdx < 0 || len(m.cycleMatches) == 0 || !strings.HasPrefix(strings.ToLower(m.cycleMatches[m.cycleIdx]), strings.ToLower(prefix)) {
		m.cycleMatches = nil
		m.cycleIdx = -1

		// List executables from PATH
		pathDirs := filepath.SplitList(os.Getenv("PATH"))
		seen := make(map[string]bool)
		for _, dir := range pathDirs {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				name := e.Name()
				if e.IsDir() || seen[name] {
					continue
				}
				if prefix == "" || strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
					m.cycleMatches = append(m.cycleMatches, "!"+name)
					seen[name] = true
				}
			}
		}
	}

	if len(m.cycleMatches) == 0 {
		return m, nil
	}

	m.cycleIdx = (m.cycleIdx + 1) % len(m.cycleMatches)
	m.input.SetValue(m.cycleMatches[m.cycleIdx])
	m.input.CursorEnd()
	return m, nil
}

// ---------------------------------------------------------------------------
// Help & tools display
// ---------------------------------------------------------------------------
