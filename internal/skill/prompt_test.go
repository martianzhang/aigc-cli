package skill

import (
	"strings"
	"testing"
)

func TestPromptSection(t *testing.T) {
	skills := []Skill{
		{Name: "alpha", Description: "Use <alpha> & friends", Path: "/tmp/a/SKILL.md"},
		{Name: "hidden", Description: "Hidden", Path: "/tmp/h/SKILL.md", DisableModelInvocation: true},
	}
	got := PromptSection(skills)

	for _, want := range []string{
		"<available_skills>",
		"<name>alpha</name>",
		"<description>Use &lt;alpha&gt; &amp; friends</description>",
		"<location>/tmp/a/SKILL.md</location>",
		"Use the read tool",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "hidden") {
		t.Errorf("disable-model-invocation skill leaked into the catalog:\n%s", got)
	}
}

func TestPromptSection_Empty(t *testing.T) {
	if got := PromptSection(nil); got != "" {
		t.Errorf("PromptSection(nil) = %q, want empty", got)
	}
	hidden := []Skill{{Name: "hidden", DisableModelInvocation: true}}
	if got := PromptSection(hidden); got != "" {
		t.Errorf("PromptSection(hidden) = %q, want empty", got)
	}
}

func TestRender(t *testing.T) {
	s := Skill{
		Name:    "pdf-tools",
		Path:    "/skills/pdf-tools/SKILL.md",
		BaseDir: "/skills/pdf-tools",
		Body:    "Read references/formats.md first.",
	}
	got := Render(s, "extract report.pdf")
	for _, want := range []string{
		`<skill name="pdf-tools" location="/skills/pdf-tools/SKILL.md">`,
		"References are relative to /skills/pdf-tools.",
		"Read references/formats.md first.",
		"</skill>",
		"extract report.pdf",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.HasSuffix(got, "\n") {
		t.Error("rendered block should not end with a newline")
	}
}

func TestRender_NoArgs(t *testing.T) {
	got := Render(Skill{Name: "a", Path: "/a/SKILL.md", BaseDir: "/a", Body: "body"}, "   ")
	if strings.HasSuffix(got, "\n\n   ") {
		t.Errorf("empty args should be omitted:\n%s", got)
	}
}

func TestExpandCommand(t *testing.T) {
	skills := []Skill{{
		Name: "alpha", Path: "/a/SKILL.md", BaseDir: "/a", Body: "Alpha instructions.",
	}}

	got, ok := ExpandCommand(skills, "/skill:alpha do it")
	if !ok {
		t.Fatal("command not expanded")
	}
	if !strings.Contains(got, "Alpha instructions.") || !strings.Contains(got, "do it") {
		t.Errorf("expanded = %q", got)
	}

	if got, ok := ExpandCommand(skills, "/skill:missing"); ok || got != "/skill:missing" {
		t.Errorf("unknown skill: got %q, ok=%v", got, ok)
	}
	if got, ok := ExpandCommand(skills, "plain text"); ok || got != "plain text" {
		t.Errorf("non-command: got %q, ok=%v", got, ok)
	}
	if got, ok := ExpandCommand(skills, "/skill:"); ok || got != "/skill:" {
		t.Errorf("empty name: got %q, ok=%v", got, ok)
	}
}
