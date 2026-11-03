// Package skill discovers Agent Skills from the cross-client .agents/skills
// convention. aigc-cli deliberately does not own a separate skills directory:
// the same files are shared with other compliant agents.
package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

const (
	// SkillsRoot is the cross-client directory name from the Agent Skills
	// integration guide. Skills live in <root>/.agents/skills/<name>/SKILL.md.
	SkillsRoot = ".agents"
	// SkillsDir is the directory below SkillsRoot that holds skill folders.
	SkillsDir = "skills"
	// SkillFileName is the required entry file of a skill directory.
	SkillFileName = "SKILL.md"
	// maxScanDepth bounds directory recursion so a large tree cannot stall
	// startup. The Agent Skills guide recommends a depth of 4-6.
	maxScanDepth = 6
)

// Scope identifies where a skill was discovered.
type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
	ScopePath    Scope = "path"
)

// Skill is one discovered Agent Skill.
type Skill struct {
	Name                   string
	Description            string
	Path                   string // absolute path to SKILL.md
	BaseDir                string // directory containing SKILL.md
	Scope                  Scope
	DisableModelInvocation bool
	Body                   string // markdown instructions without frontmatter
}

// Diagnostic reports a skill that could not be loaded or that shadowed
// another skill. Diagnostics never stop discovery.
type Diagnostic struct {
	Path    string
	Message string
}

// Options controls skill discovery.
type Options struct {
	// Cwd is the project directory for .agents/skills lookup.
	Cwd string
	// Home is the user home directory for ~/.agents/skills; empty disables it.
	Home string
	// Paths are explicit skill files or directories. They load after automatic
	// discovery and override skills with the same name.
	Paths []string
	// NoDefault skips automatic user/project discovery. Explicit Paths still load.
	NoDefault bool
}

// Discover loads skills for cwd from the user and project scopes.
func Discover(cwd string) ([]Skill, []Diagnostic) {
	home, _ := os.UserHomeDir()
	return DiscoverFrom(cwd, home)
}

// DiscoverFrom is Discover with an explicit home directory, for tests.
func DiscoverFrom(cwd, home string) ([]Skill, []Diagnostic) {
	return DiscoverWithOptions(Options{Cwd: cwd, Home: home})
}

// DiscoverWithOptions loads skills from the configured scopes and paths.
//
// Automatic roots are loaded user first, then project directories from the
// outermost ancestor to the innermost, so project skills override user skills
// and a skill closer to cwd wins over a parent directory's copy. Explicit
// paths load last and always override. Skills sharing a name inside one root
// keep the first discovered (lexical) entry.
func DiscoverWithOptions(opts Options) ([]Skill, []Diagnostic) {
	byName := make(map[string]Skill)
	realPaths := make(map[string]bool)
	var diags []Diagnostic

	if !opts.NoDefault {
		for _, root := range skillRoots(opts.Cwd, opts.Home) {
			skills, rootDiags := loadRoot(root.dir, root.scope)
			diags = append(diags, rootDiags...)
			diags = mergeSkills(byName, realPaths, diags, skills, root.override)
		}
	}

	for _, raw := range opts.Paths {
		path := expandHome(strings.TrimSpace(raw), opts.Home)
		if path == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			diags = append(diags, Diagnostic{Path: path, Message: err.Error()})
			continue
		}
		skills, pathDiags := loadExplicitPath(absolute)
		diags = append(diags, pathDiags...)
		diags = mergeSkills(byName, realPaths, diags, skills, true)
	}

	out := make([]Skill, 0, len(byName))
	for _, s := range byName {
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b Skill) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out, diags
}

type root struct {
	dir      string
	scope    Scope
	override bool
}

func skillRoots(cwd, home string) []root {
	var roots []root
	userDir := ""
	if home != "" {
		userDir = filepath.Join(home, SkillsRoot, SkillsDir)
		roots = append(roots, root{dir: userDir, scope: ScopeUser})
	}
	if cwd == "" {
		return roots
	}

	// Collect cwd first, then walk up; reverse so outer roots load first and
	// the most specific project directory wins. The user-level directory is
	// skipped here so a cwd under $HOME does not load it twice.
	var project []root
	for dir := filepath.Clean(cwd); ; {
		skillDir := filepath.Join(dir, SkillsRoot, SkillsDir)
		if userDir == "" || !samePath(skillDir, userDir) {
			project = append(project, root{dir: skillDir, scope: ScopeProject, override: true})
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
	slices.Reverse(project)
	return append(roots, project...)
}

// samePath compares two cleaned paths. Windows paths are case-insensitive, so
// a differently-cased user-home path is still recognised as the same root.
func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// isRepoRoot reports whether dir contains a .git entry (file or directory).
func isRepoRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// loadRoot scans one skills directory. A directory containing SKILL.md is a
// skill and is not traversed further; other directories are visited
// recursively. Hidden directories and node_modules are skipped.
func loadRoot(dir string, scope Scope) ([]Skill, []Diagnostic) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	var (
		skills  []Skill
		diags   []Diagnostic
		seen    = make(map[string]bool)
		visited = make(map[string]bool)
	)

	var walk func(string, int)
	walk = func(current string, depth int) {
		if depth > maxScanDepth {
			return
		}
		real, err := filepath.EvalSymlinks(current)
		if err != nil {
			real = current
		}
		if visited[real] {
			return
		}
		visited[real] = true

		skillPath := filepath.Join(current, SkillFileName)
		if fi, err := os.Stat(skillPath); err == nil && fi.Mode().IsRegular() {
			s, skillDiags := loadSkillFile(skillPath, scope)
			diags = append(diags, skillDiags...)
			if s != nil {
				if seen[s.Name] {
					diags = append(diags, Diagnostic{
						Path:    s.Path,
						Message: fmt.Sprintf("duplicate skill %q in %s; keeping the first one", s.Name, dir),
					})
				} else {
					seen[s.Name] = true
					skills = append(skills, *s)
				}
			}
			return
		}

		entries, err := os.ReadDir(current)
		if err != nil {
			diags = append(diags, Diagnostic{Path: current, Message: err.Error()})
			return
		}
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" {
				continue
			}
			child := filepath.Join(current, name)
			fi, err := os.Stat(child)
			if err != nil || !fi.IsDir() {
				continue
			}
			walk(child, depth+1)
		}
	}

	walk(dir, 0)
	return skills, diags
}

// mergeSkills inserts skills into byName, skipping files already loaded and
// reporting name collisions. When override is false, an existing skill wins.
func mergeSkills(byName map[string]Skill, realPaths map[string]bool, diags []Diagnostic, skills []Skill, override bool) []Diagnostic {
	for _, s := range skills {
		key := canonicalPath(s.Path)
		if realPaths[key] {
			continue
		}
		previous, exists := byName[s.Name]
		if exists {
			diags = append(diags, Diagnostic{
				Path: s.Path,
				Message: fmt.Sprintf("duplicate skill %q; %s overrides %s",
					s.Name, s.Path, previous.Path),
			})
			if !override {
				continue
			}
		}
		byName[s.Name] = s
		realPaths[key] = true
	}
	return diags
}

// loadExplicitPath loads one --skill / skills.paths entry. A markdown file is
// a single skill; a directory is scanned for SKILL.md and, at its root, for
// standalone .md skills.
func loadExplicitPath(path string) ([]Skill, []Diagnostic) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, []Diagnostic{{Path: path, Message: "skill path does not exist"}}
	}
	if info.IsDir() {
		return loadExplicitDir(path)
	}
	if !info.Mode().IsRegular() {
		return nil, []Diagnostic{{Path: path, Message: "skill path is not a regular file"}}
	}
	if !strings.HasSuffix(strings.ToLower(path), ".md") {
		return nil, []Diagnostic{{Path: path, Message: "skill path is not a markdown file"}}
	}
	s, diags := loadSkillFile(path, ScopePath)
	if s == nil {
		return nil, diags
	}
	return []Skill{*s}, diags
}

func loadExplicitDir(dir string) ([]Skill, []Diagnostic) {
	// A directory containing SKILL.md is one skill; do not also load siblings.
	if fi, err := os.Stat(filepath.Join(dir, SkillFileName)); err == nil && fi.Mode().IsRegular() {
		return loadRoot(dir, ScopePath)
	}

	skills, diags := loadRoot(dir, ScopePath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return skills, append(diags, Diagnostic{Path: dir, Message: err.Error()})
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == SkillFileName ||
			!strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		full := filepath.Join(dir, name)
		fi, err := os.Stat(full)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		s, skillDiags := loadSkillFile(full, ScopePath)
		diags = append(diags, skillDiags...)
		if s != nil {
			skills = appendUniqueSkill(skills, *s)
		}
	}
	return skills, diags
}

func appendUniqueSkill(skills []Skill, s Skill) []Skill {
	for _, existing := range skills {
		if existing.Name == s.Name {
			return skills
		}
	}
	return append(skills, s)
}

func canonicalPath(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return filepath.Clean(path)
}

// expandHome expands a leading ~ or ~/ in a configured skill path.
func expandHome(path, home string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

// Find returns the skill with the given name.
func Find(skills []Skill, name string) (Skill, bool) {
	for _, s := range skills {
		if s.Name == name {
			return s, true
		}
	}
	return Skill{}, false
}

// Names returns the sorted skill names, for error messages.
func Names(skills []Skill) []string {
	names := make([]string, 0, len(skills))
	for _, s := range skills {
		names = append(names, s.Name)
	}
	slices.Sort(names)
	return names
}
