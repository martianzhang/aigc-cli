package chat

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/martianzhang/aigc-cli/internal/skill"
)

// handleSkillCommand expands /skill:<name> [args] into the skill instructions
// and sends it as the next user turn. The transcript keeps the command the
// user typed; the model receives the expanded block.
func (m *chatModel) handleSkillCommand(input string) (tea.Model, tea.Cmd) {
	expanded, errMessage, ok := resolveSkillCommand(input)
	if !ok {
		m.appendSystemMessage(errMessage)
		return m, nil
	}
	return m.startUserTurn(input, expanded, nil, nil)
}

// resolveSkillCommand turns /skill:<name> [args] into the instruction block the
// model receives. On failure it returns a user-facing error message.
func resolveSkillCommand(input string) (expanded, errMessage string, ok bool) {
	if chatNoSkills {
		return "", "Skills are disabled by --no-skills.", false
	}
	name, args := parseSkillInvocation(strings.TrimPrefix(input, "/skill:"))
	if name == "" {
		return "", "Usage: /skill:<name> [args]", false
	}

	skills, diags := discoverSkills()
	logSkillDiagnostics(diags)

	s, found := skill.Find(skills, name)
	if !found {
		message := fmt.Sprintf("Unknown skill %q.", name)
		if names := skill.Names(skills); len(names) > 0 {
			message += " Available: " + strings.Join(names, ", ")
		} else {
			message += " No skills found in .agents/skills or ~/.agents/skills."
		}
		return "", message, false
	}

	return skill.Render(s, args), "", true
}

// showSkills lists the skills discovered for the current working directory.
func (m *chatModel) showSkills() {
	if chatNoSkills {
		m.appendSystemMessage("Skills are disabled by --no-skills.")
		return
	}
	skills, diags := discoverSkills()
	logSkillDiagnostics(diags)
	m.appendSystemMessage(formatSkillList(skills))
}

// formatSkillList renders the /skills view: one entry per loaded skill with
// its scope, explicit-invocation marker, and description.
func formatSkillList(skills []skill.Skill) string {
	if len(skills) == 0 {
		return "No skills found.\n\n" +
			"Create ~/.agents/skills/<name>/SKILL.md or .agents/skills/<name>/SKILL.md, " +
			"or pass --skill <path>."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Loaded skills (%d):\n", len(skills))
	for _, s := range skills {
		tag := string(s.Scope)
		if s.DisableModelInvocation {
			tag += ", manual-only"
		}
		description := collapsedSummary(s.Description, 160)
		if description == "" {
			description = "(no description)"
		}
		fmt.Fprintf(&b, "\n  %s  [%s]\n    %s\n", s.Name, tag, description)
	}
	b.WriteString("\nUse /skill:<name> [args] to load one explicitly.")
	return b.String()
}

// parseSkillInvocation splits the text after /skill: into a skill name and
// the optional arguments.
func parseSkillInvocation(rest string) (name, args string) {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", ""
	}
	if idx := strings.IndexAny(rest, " \t\n"); idx >= 0 {
		return rest[:idx], strings.TrimSpace(rest[idx+1:])
	}
	return rest, ""
}

func (m *chatModel) appendSystemMessage(text string) {
	m.messages = append(m.messages, message{role: "system", content: text})
	m.refreshViewport()
	m.input.Reset()
}
