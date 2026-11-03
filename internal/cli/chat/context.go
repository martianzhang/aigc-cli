package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/skill"
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
// platform/shell notes (only on platforms where it matters), the AGENTS.md
// project context, and the available Agent Skills catalog.
func runtimeContext() string {
	var parts []string
	if p := agent.PlatformPrompt(); p != "" {
		parts = append(parts, p)
	}
	if ctx := loadAgentsContext(); ctx != "" {
		parts = append(parts, ctx)
	}
	if ctx := skillsContext(); ctx != "" {
		parts = append(parts, ctx)
	}
	return strings.Join(parts, "\n\n")
}

// skillsContext builds the skill catalog for the system prompt. Skills are
// loaded through the read tool, so the catalog is omitted when that tool is
// disabled: advertising skills the model cannot open only wastes tokens.
func skillsContext() string {
	if !hasTool("read") {
		return ""
	}
	skills, diags := discoverSkills()
	logSkillDiagnostics(diags)
	return skill.PromptSection(skills)
}

// skillHomeDir is os.UserHomeDir in production; tests override it so discovery
// never touches the real user profile.
var skillHomeDir = os.UserHomeDir

// discoverSkills returns the skills visible from the current working directory,
// combining config paths, --skill paths, and automatic discovery.
func discoverSkills() ([]skill.Skill, []skill.Diagnostic) {
	if chatNoSkills {
		return nil, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil
	}
	home, _ := skillHomeDir()

	cfg := options.SkillsConfig()
	enabled := cfg == nil || cfg.Enabled == nil || *cfg.Enabled

	var paths []string
	if enabled && cfg != nil {
		paths = append(paths, cfg.Paths...)
	}
	paths = append(paths, chatSkillPaths...)

	return skill.DiscoverWithOptions(skill.Options{
		Cwd:       cwd,
		Home:      home,
		Paths:     paths,
		NoDefault: !enabled,
	})
}

// logSkillDiagnostics prints discovery warnings only in verbose mode so normal
// chat output stays clean.
func logSkillDiagnostics(diags []skill.Diagnostic) {
	if !options.Shared.Verbose {
		return
	}
	for _, d := range diags {
		fmt.Fprintf(options.Stderr(), "skill: %s: %s\n", d.Path, d.Message)
	}
}

// hasTool reports whether the effective built-in tool set contains name.
func hasTool(name string) bool {
	for _, t := range buildAgentTools(options.ChatDefaults()) {
		if t.Function.Name == name {
			return true
		}
	}
	return false
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
