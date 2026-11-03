package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, dir, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, SkillFileName)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func skillMD(name, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\n\nUse this skill.\n"
}

func writeMarkdown(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverFrom(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "sub")

	writeSkill(t, filepath.Join(home, ".agents", "skills", "user-skill"),
		skillMD("user-skill", "User level skill"))
	writeSkill(t, filepath.Join(root, ".agents", "skills", "outer-skill"),
		skillMD("outer-skill", "Repository root skill"))
	writeSkill(t, filepath.Join(project, ".agents", "skills", "project-skill"),
		skillMD("project-skill", "Project skill"))
	writeSkill(t, filepath.Join(project, ".agents", "skills", "nested", "deep"),
		skillMD("deep-skill", "Nested skill"))
	writeSkill(t, filepath.Join(project, ".agents", "skills", ".hidden"),
		skillMD("hidden-skill", "Must be ignored"))
	writeSkill(t, filepath.Join(project, ".agents", "skills", "node_modules", "pkg"),
		skillMD("dep-skill", "Must be ignored"))

	// A project skill shadows the user skill with the same name.
	writeSkill(t, filepath.Join(home, ".agents", "skills", "shared"),
		skillMD("shared", "User copy"))
	writeSkill(t, filepath.Join(project, ".agents", "skills", "shared"),
		skillMD("shared", "Project copy"))

	// Missing description: reported, not loaded.
	writeSkill(t, filepath.Join(project, ".agents", "skills", "bad"), "---\nname: bad\n---\n")

	skills, diags := DiscoverFrom(project, home)

	got := Names(skills)
	want := []string{"deep-skill", "outer-skill", "project-skill", "shared", "user-skill"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("names = %v, want %v", got, want)
	}

	shared, ok := Find(skills, "shared")
	if !ok {
		t.Fatal("shared skill missing")
	}
	if shared.Description != "Project copy" || shared.Scope != ScopeProject {
		t.Errorf("shared = %+v, want project copy", shared)
	}
	if user, _ := Find(skills, "user-skill"); user.Scope != ScopeUser {
		t.Errorf("user-skill scope = %q, want %q", user.Scope, ScopeUser)
	}
	if _, ok := Find(skills, "hidden-skill"); ok {
		t.Error("hidden directory skill should be ignored")
	}
	if _, ok := Find(skills, "dep-skill"); ok {
		t.Error("node_modules skill should be ignored")
	}

	var sawMissing, sawCollision bool
	for _, d := range diags {
		if strings.Contains(d.Message, "description is required") {
			sawMissing = true
		}
		if strings.Contains(d.Message, "duplicate skill") {
			sawCollision = true
		}
	}
	if !sawMissing {
		t.Error("missing description diagnostic not reported")
	}
	if !sawCollision {
		t.Error("collision diagnostic not reported")
	}
}

func TestDiscoverFrom_StopsAtRepoRoot(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "sub")
	writeSkill(t, filepath.Join(root, ".agents", "skills", "root-skill"),
		skillMD("root-skill", "Root skill"))

	// A skills directory above the repository must not be loaded.
	writeSkill(t, filepath.Join(filepath.Dir(root), ".agents", "skills", "outside-skill"),
		skillMD("outside-skill", "Outside skill"))

	skills, _ := DiscoverFrom(project, home)
	if _, ok := Find(skills, "root-skill"); !ok {
		t.Error("root skill should be loaded")
	}
	if _, ok := Find(skills, "outside-skill"); ok {
		t.Error("skill above the repository root should not be loaded")
	}
}

func TestDiscoverFrom_NoCwd(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, filepath.Join(home, ".agents", "skills", "user-skill"),
		skillMD("user-skill", "User level skill"))

	skills, diags := DiscoverFrom("", home)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if len(skills) != 1 || skills[0].Name != "user-skill" {
		t.Fatalf("skills = %+v, want user-skill", skills)
	}
}

func TestDiscoverFrom_CwdUnderHomeDoesNotDuplicateUserRoot(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(home, ".agents", "skills", "user-skill"),
		skillMD("user-skill", "User level skill"))
	cwd := filepath.Join(home, "project")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	skills, diags := DiscoverFrom(cwd, home)
	count := 0
	for _, s := range skills {
		if s.Name == "user-skill" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("user-skill loaded %d times, want 1", count)
	}
	for _, d := range diags {
		if strings.Contains(d.Message, "duplicate") {
			t.Errorf("unexpected duplicate diagnostic: %+v", d)
		}
	}
}

func TestDiscoverWithOptions_ExplicitPaths(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, filepath.Join(home, ".agents", "skills", "user-skill"),
		skillMD("user-skill", "User level skill"))

	explicitFile := filepath.Join(t.TempDir(), "custom.md")
	writeMarkdown(t, explicitFile, skillMD("custom", "Explicit file skill"))

	collection := t.TempDir()
	writeSkill(t, filepath.Join(collection, "nested"), skillMD("nested", "Nested skill"))
	writeMarkdown(t, filepath.Join(collection, "loose.md"), skillMD("loose", "Loose markdown skill"))

	skills, diags := DiscoverWithOptions(Options{
		Cwd:       t.TempDir(),
		Home:      home,
		Paths:     []string{explicitFile, collection},
		NoDefault: true,
	})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	got := Names(skills)
	want := []string{"custom", "loose", "nested"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("names = %v, want %v", got, want)
	}
	for _, s := range skills {
		if s.Scope != ScopePath {
			t.Errorf("%s scope = %q, want %q", s.Name, s.Scope, ScopePath)
		}
	}
}

func TestDiscoverWithOptions_ExplicitOverrides(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, filepath.Join(home, ".agents", "skills", "shared"),
		skillMD("shared", "User copy"))

	explicit := filepath.Join(t.TempDir(), "shared.md")
	writeMarkdown(t, explicit, skillMD("shared", "Explicit copy"))

	cwd := t.TempDir()
	if err := os.Mkdir(filepath.Join(cwd, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	skills, _ := DiscoverWithOptions(Options{
		Cwd:   cwd,
		Home:  home,
		Paths: []string{explicit},
	})
	shared, ok := Find(skills, "shared")
	if !ok {
		t.Fatal("shared skill missing")
	}
	if shared.Description != "Explicit copy" || shared.Scope != ScopePath {
		t.Errorf("shared = %+v, want explicit copy", shared)
	}
}

func TestDiscoverWithOptions_NoDefault(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, filepath.Join(home, ".agents", "skills", "user-skill"),
		skillMD("user-skill", "User level skill"))

	skills, diags := DiscoverWithOptions(Options{
		Cwd:       t.TempDir(),
		Home:      home,
		NoDefault: true,
	})
	if len(skills) != 0 || len(diags) != 0 {
		t.Fatalf("skills=%v diags=%v, want empty", skills, diags)
	}
}

func TestDiscoverWithOptions_ExplicitPathExpandsHome(t *testing.T) {
	home := t.TempDir()
	writeMarkdown(t, filepath.Join(home, "skills", "custom.md"),
		skillMD("custom", "Home relative skill"))

	skills, diags := DiscoverWithOptions(Options{
		Home:      home,
		Paths:     []string{"~/skills/custom.md"},
		NoDefault: true,
	})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if len(skills) != 1 || skills[0].Name != "custom" {
		t.Fatalf("skills = %v, want custom", Names(skills))
	}
}

func TestDiscoverWithOptions_ExplicitPathDiagnostics(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	notMarkdown := filepath.Join(t.TempDir(), "skill.txt")
	writeMarkdown(t, notMarkdown, "not a skill")

	skills, diags := DiscoverWithOptions(Options{NoDefault: true, Paths: []string{missing, notMarkdown}})
	if len(skills) != 0 {
		t.Fatalf("skills = %v, want empty", skills)
	}
	var sawMissing, sawNotMarkdown bool
	for _, d := range diags {
		if strings.Contains(d.Message, "does not exist") {
			sawMissing = true
		}
		if strings.Contains(d.Message, "not a markdown file") {
			sawNotMarkdown = true
		}
	}
	if !sawMissing || !sawNotMarkdown {
		t.Errorf("diagnostics = %+v", diags)
	}
}

func TestDiscoverWithOptions_DirectoryWithRootSkillIgnoresSiblings(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, skillMD("root-skill", "Root skill"))
	writeMarkdown(t, filepath.Join(dir, "sibling.md"), skillMD("sibling", "Sibling skill"))

	skills, _ := DiscoverWithOptions(Options{NoDefault: true, Paths: []string{dir}})
	if len(skills) != 1 || skills[0].Name != "root-skill" {
		t.Fatalf("skills = %v, want only root-skill", Names(skills))
	}
}

func TestDiscoverFrom_MissingRoots(t *testing.T) {
	cwd := t.TempDir()
	if err := os.Mkdir(filepath.Join(cwd, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	skills, diags := DiscoverFrom(cwd, t.TempDir())
	if len(skills) != 0 || len(diags) != 0 {
		t.Fatalf("skills=%v diags=%v, want empty", skills, diags)
	}
}
