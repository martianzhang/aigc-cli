package chat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/skill"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func writeChatSkill(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, ".agents", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: Demo skill. Use when testing.\n---\n\n" + body
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setSkillHome isolates discovery from the real user profile. os.UserHomeDir
// uses USERPROFILE on Windows and HOME elsewhere, so overriding the function
// keeps tests portable.
func setSkillHome(t *testing.T, home string) {
	t.Helper()
	previous := skillHomeDir
	skillHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { skillHomeDir = previous })
}

func TestRuntimeContextIncludesSkills(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeChatSkill(t, project, "demo", "Demo body.")
	t.Chdir(project)
	setSkillHome(t, t.TempDir())

	prev := options.Shared.Cfg
	options.Shared.Cfg = nil
	t.Cleanup(func() { options.Shared.Cfg = prev })

	got := runtimeContext()
	for _, want := range []string{"<available_skills>", "<name>demo</name>", "Demo skill. Use when testing."} {
		if !strings.Contains(got, want) {
			t.Errorf("runtimeContext missing %q:\n%s", want, got)
		}
	}

	// With read disabled the catalog must not be advertised.
	options.Shared.Cfg = &types.Config{ToolsDisable: []string{"read"}}
	if got := runtimeContext(); strings.Contains(got, "<available_skills>") {
		t.Errorf("skills advertised without the read tool:\n%s", got)
	}
}

func TestExpandUserInputSkillCommand(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeChatSkill(t, project, "demo", "Demo body.")
	t.Chdir(project)
	setSkillHome(t, t.TempDir())

	got, images, warnings := expandUserInput("/skill:demo do the thing", nil)
	if !strings.Contains(got, "Demo body.") || !strings.Contains(got, "do the thing") {
		t.Errorf("expanded = %q", got)
	}
	if len(images) != 0 || len(warnings) != 0 {
		t.Errorf("images=%v warnings=%v", images, warnings)
	}
}

func TestDiscoverSkillsHonorsConfigAndFlags(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeChatSkill(t, project, "auto", "Auto body.")
	explicit := filepath.Join(t.TempDir(), "manual.md")
	if err := os.WriteFile(explicit, []byte(
		"---\nname: manual\ndescription: Manual skill.\n---\nManual body."), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)
	setSkillHome(t, t.TempDir())

	prevCfg := options.Shared.Cfg
	prevPaths := chatSkillPaths
	prevNoSkills := chatNoSkills
	t.Cleanup(func() {
		options.Shared.Cfg = prevCfg
		chatSkillPaths = prevPaths
		chatNoSkills = prevNoSkills
	})

	disabled := false
	options.Shared.Cfg = &types.Config{Skills: &types.SkillsConfig{Paths: []string{explicit}}}
	chatSkillPaths = nil
	chatNoSkills = false

	skills, _ := discoverSkills()
	if len(skills) != 2 {
		t.Fatalf("skills = %v, want auto + config path", skill.Names(skills))
	}

	// enabled=false ignores config paths but explicit --skill still loads.
	options.Shared.Cfg.Skills.Enabled = &disabled
	skills, _ = discoverSkills()
	if len(skills) != 0 {
		t.Fatalf("skills = %v, want none when disabled", skill.Names(skills))
	}
	chatSkillPaths = []string{explicit}
	skills, _ = discoverSkills()
	if len(skills) != 1 || skills[0].Name != "manual" {
		t.Fatalf("skills = %v, want manual", skill.Names(skills))
	}

	// --no-skills disables discovery and loading entirely, including --skill.
	options.Shared.Cfg.Skills.Enabled = nil
	chatNoSkills = true
	skills, _ = discoverSkills()
	if len(skills) != 0 {
		t.Fatalf("skills = %v, want none", skill.Names(skills))
	}
}

func TestCommandCompletions(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeChatSkill(t, project, "demo", "Demo body.")
	t.Chdir(project)
	setSkillHome(t, t.TempDir())

	got := strings.Join(commandCompletions(nil), ",")
	for _, want := range []string{"/skills", "/compact", "/preview", "/skill:demo"} {
		if !strings.Contains(got, want) {
			t.Errorf("completions missing %q: %s", want, got)
		}
	}
}

func TestFormatSkillList(t *testing.T) {
	empty := formatSkillList(nil)
	if !strings.Contains(empty, "No skills found") ||
		!strings.Contains(empty, "~/.agents/skills") {
		t.Errorf("empty list = %q", empty)
	}

	got := formatSkillList([]skill.Skill{
		{Name: "demo", Description: "Demo skill.", Scope: skill.ScopeProject},
		{Name: "manual", Description: "Manual only.", Scope: skill.ScopeUser, DisableModelInvocation: true},
	})
	for _, want := range []string{
		"Loaded skills (2)",
		"demo  [project]",
		"Demo skill.",
		"manual  [user, manual-only]",
		"Manual only.",
		"/skill:<name>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestResolveSkillCommand(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeChatSkill(t, project, "demo", "Demo body.")
	t.Chdir(project)
	setSkillHome(t, t.TempDir())

	expanded, errMessage, ok := resolveSkillCommand("/skill:demo go")
	if !ok {
		t.Fatalf("resolveSkillCommand failed: %s", errMessage)
	}
	if !strings.Contains(expanded, "Demo body.") || !strings.Contains(expanded, "go") {
		t.Errorf("expanded = %q", expanded)
	}

	if _, message, ok := resolveSkillCommand("/skill:missing"); ok ||
		!strings.Contains(message, "Unknown skill") {
		t.Errorf("missing skill: ok=%v message=%q", ok, message)
	}
	if _, message, ok := resolveSkillCommand("/skill:"); ok ||
		!strings.Contains(message, "Usage") {
		t.Errorf("empty name: ok=%v message=%q", ok, message)
	}
}

func TestParseSkillInvocation(t *testing.T) {
	tests := []struct {
		in       string
		wantName string
		wantArgs string
	}{
		{in: "demo", wantName: "demo"},
		{in: "demo arg", wantName: "demo", wantArgs: "arg"},
		{in: "  demo   a b  ", wantName: "demo", wantArgs: "a b"},
		{in: "demo\nmulti line", wantName: "demo", wantArgs: "multi line"},
		{in: "", wantName: "", wantArgs: ""},
	}
	for _, tc := range tests {
		name, args := parseSkillInvocation(tc.in)
		if name != tc.wantName || args != tc.wantArgs {
			t.Errorf("parseSkillInvocation(%q) = (%q, %q), want (%q, %q)",
				tc.in, name, args, tc.wantName, tc.wantArgs)
		}
	}
}
