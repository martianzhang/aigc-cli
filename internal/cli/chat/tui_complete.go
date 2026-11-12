package chat

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// commandCompletions returns the /-prefixed candidates Tab cycles through.
// Skill names come from the same discovery used by /skills and /skill:<name>.
func commandCompletions(tools []types.ToolDefinition) []string {
	completions := []string{
		"/exit", "/quit", "/q",
		"/clear", "/reset", "/new",
		"/help", "/?", "/tools", "/skills", "/model", "/provider", "/context", "/compact",
		"/copy", "/name", "/preview", "/save", "/session", "/sessions", "/resume",
	}
	for _, t := range tools {
		completions = append(completions, "/"+t.Function.Name)
	}
	skills, _ := discoverSkills()
	for _, s := range skills {
		completions = append(completions, "/skill:"+s.Name)
	}
	return completions
}

// handleTabCompletion completes commands (/xxx), shell commands (!xxx), or
// @file mentions.
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

	// File mention completion: trailing @<fragment>
	if start := trailingMentionStart(input); start >= 0 {
		return m.completeMention(input, start)
	}

	// Argument completion for the provider/model switch commands.
	if strings.HasPrefix(input, "/provider ") {
		return m.completeProviderArgument(input)
	}
	if strings.HasPrefix(input, "/model ") {
		return m.completeModelArgument(input)
	}

	// Command completion: /<cmd>
	return m.completeCommand(input)
}

func (m *chatModel) completeProviderArgument(input string) (tea.Model, tea.Cmd) {
	prefix := strings.TrimSpace(strings.TrimPrefix(input, "/provider "))
	candidates := make([]string, 0, len(options.ProviderNames())+1)
	for _, name := range options.ProviderNames() {
		if hasFoldPrefix(name, prefix) {
			candidates = append(candidates, "/provider "+name)
		}
	}
	if hasFoldPrefix("default", prefix) {
		candidates = append(candidates, "/provider default")
	}
	return m.cycleCandidates(input, candidates)
}

func (m *chatModel) completeModelArgument(input string) (tea.Model, tea.Cmd) {
	rest := strings.TrimSpace(strings.TrimPrefix(input, "/model "))
	fields := strings.Fields(rest)
	providerName := m.provider
	modelPrefix := ""
	fullPrefix := "/model "
	switch {
	case len(fields) >= 2:
		providerName = fields[0]
		modelPrefix = fields[1]
		fullPrefix = "/model " + providerName + " "
	case len(fields) == 1:
		if isConfiguredProvider(fields[0]) {
			providerName = fields[0]
			fullPrefix = "/model " + providerName + " "
		} else {
			modelPrefix = fields[0]
		}
	}

	models, err := modelListForProvider(providerName, modelContextWindowProbeTimeout)
	values := make([]string, 0, 1)
	if err == nil {
		models = append([]types.OpenAIModel(nil), models...)
		sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
		for _, model := range models {
			if len(values) >= 200 {
				break
			}
			if hasFoldPrefix(model.ID, modelPrefix) {
				values = append(values, fullPrefix+model.ID)
			}
		}
	}
	if hasFoldPrefix("default", modelPrefix) {
		values = append(values, fullPrefix+"default")
	}
	return m.cycleCandidates(input, values)
}

func (m *chatModel) cycleCandidates(input string, candidates []string) (tea.Model, tea.Cmd) {
	if len(candidates) == 0 {
		return m, nil
	}
	if m.cycleIdx < 0 || len(m.cycleMatches) == 0 ||
		m.cycleIdx >= len(m.cycleMatches) ||
		!strings.HasPrefix(strings.ToLower(m.cycleMatches[m.cycleIdx]), strings.ToLower(input)) {
		m.cycleMatches = candidates
		m.cycleIdx = -1
	}
	m.cycleIdx = (m.cycleIdx + 1) % len(m.cycleMatches)
	m.input.SetValue(m.cycleMatches[m.cycleIdx])
	m.input.CursorEnd()
	return m, nil
}

func hasFoldPrefix(value, prefix string) bool {
	return strings.HasPrefix(strings.ToLower(value), strings.ToLower(prefix))
}

func isConfiguredProvider(name string) bool {
	for _, configured := range options.ProviderNames() {
		if configured == name {
			return true
		}
	}
	return false
}

// trailingMentionStart returns the index of the '@' that begins the trailing
// (unterminated, whitespace-free) token, or -1 when the cursor is not inside an
// @-mention.
func trailingMentionStart(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		switch {
		case isMentionSpace(s[i]):
			return -1
		case s[i] == '@':
			if i == 0 || isMentionSpace(s[i-1]) {
				return i
			}
			return -1
		}
	}
	return -1
}

// completeMention cycles project files matching the @-fragment on Tab, replacing
// the trailing mention with the selected path.
func (m *chatModel) completeMention(input string, start int) (tea.Model, tea.Cmd) {
	if !m.fileIndexReady {
		m.fileIndex = projectFiles(".")
		m.fileIndexReady = true
	}

	fragment := input[start+1:]
	// Reset cycle if the fragment changed since the last Tab.
	if m.cycleIdx < 0 || len(m.cycleMatches) == 0 ||
		!strings.HasPrefix(strings.TrimPrefix(m.cycleMatches[m.cycleIdx], "@"), fragment) {
		m.cycleMatches = nil
		lower := strings.ToLower(fragment)
		for _, file := range m.fileIndex {
			if strings.Contains(strings.ToLower(file), lower) {
				m.cycleMatches = append(m.cycleMatches, "@"+file)
			}
		}
		m.cycleIdx = -1
	}

	if len(m.cycleMatches) == 0 {
		return m, nil
	}

	m.cycleIdx = (m.cycleIdx + 1) % len(m.cycleMatches)
	m.input.SetValue(input[:start] + m.cycleMatches[m.cycleIdx])
	m.input.CursorEnd()
	return m, nil
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
