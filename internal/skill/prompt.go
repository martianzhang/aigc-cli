package skill

import (
	"fmt"
	"strings"
)

var xmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&apos;",
)

// PromptSection formats the skill catalog for the system prompt. Only names,
// descriptions, and locations are included; full instructions stay on disk
// until the model reads the file (progressive disclosure).
//
// Skills with disable-model-invocation are omitted: they remain available
// through /skill:<name> but are hidden from automatic selection.
func PromptSection(skills []Skill) string {
	visible := make([]Skill, 0, len(skills))
	for _, s := range skills {
		if s.DisableModelInvocation {
			continue
		}
		visible = append(visible, s)
	}
	if len(visible) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("The following skills provide specialized instructions for specific tasks.\n")
	b.WriteString("Use the read tool to load a skill's file when the task matches its description.\n")
	b.WriteString("Resolve relative paths against the skill's directory. Use absolute paths in tool calls.\n\n")
	b.WriteString("<available_skills>")
	for _, s := range visible {
		fmt.Fprintf(&b, "\n  <skill>\n")
		fmt.Fprintf(&b, "    <name>%s</name>\n", xmlEscaper.Replace(s.Name))
		fmt.Fprintf(&b, "    <description>%s</description>\n", xmlEscaper.Replace(s.Description))
		fmt.Fprintf(&b, "    <location>%s</location>\n", xmlEscaper.Replace(s.Path))
		fmt.Fprintf(&b, "  </skill>")
	}
	b.WriteString("\n</available_skills>")
	return b.String()
}

// Render expands a skill into the block Pi uses for explicit invocation:
// the full instructions plus a pointer to the skill directory.
func Render(s Skill, args string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<skill name=\"%s\" location=\"%s\">\n",
		xmlEscaper.Replace(s.Name), xmlEscaper.Replace(s.Path))
	fmt.Fprintf(&b, "References are relative to %s.\n\n", xmlEscaper.Replace(s.BaseDir))
	b.WriteString(strings.TrimSpace(s.Body))
	b.WriteString("\n</skill>")

	if args = strings.TrimSpace(args); args != "" {
		b.WriteString("\n\n")
		b.WriteString(args)
	}
	return b.String()
}

// ExpandCommand expands an explicit /skill:<name> [args] prompt. Unknown or
// malformed commands are returned unchanged so the text still reaches the
// model as ordinary input.
func ExpandCommand(skills []Skill, input string) (string, bool) {
	const prefix = "/skill:"
	if !strings.HasPrefix(input, prefix) {
		return input, false
	}

	rest := strings.TrimPrefix(input, prefix)
	name, args := rest, ""
	if idx := strings.IndexAny(rest, " \t\n"); idx >= 0 {
		name = rest[:idx]
		args = rest[idx+1:]
	}
	s, ok := Find(skills, name)
	if !ok {
		return input, false
	}
	return Render(s, args), true
}
