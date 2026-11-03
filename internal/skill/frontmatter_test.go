package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantMeta    string
		wantBody    string
		wantErrPart string
	}{
		{
			name:     "simple",
			content:  "---\nname: one\ndescription: Desc\n---\n\nBody\n",
			wantMeta: "name: one\ndescription: Desc",
			wantBody: "Body",
		},
		{
			name:     "bom and crlf",
			content:  "\ufeff---\r\nname: one\r\ndescription: Desc\r\n---\r\n\r\nBody\r\n",
			wantMeta: "name: one\ndescription: Desc",
			wantBody: "Body",
		},
		{
			name:     "delimiter with spaces",
			content:  "---  \nname: one\ndescription: Desc\n---  \nBody",
			wantMeta: "name: one\ndescription: Desc",
			wantBody: "Body",
		},
		{
			name:        "missing",
			content:     "Body only",
			wantErrPart: "missing YAML frontmatter",
		},
		{
			name:        "unterminated",
			content:     "---\nname: one\n",
			wantErrPart: "unterminated YAML frontmatter",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			meta, body, err := splitFrontmatter(tc.content)
			if tc.wantErrPart != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrPart) {
					t.Fatalf("err = %v, want contains %q", err, tc.wantErrPart)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if meta != tc.wantMeta {
				t.Errorf("meta = %q, want %q", meta, tc.wantMeta)
			}
			if body != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}

func TestLoadSkillFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my-skill", SkillFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ndescription: A useful skill\n---\n\n# Instructions\n\nDo the thing.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	s, diags := loadSkillFile(path, ScopeProject)
	if s == nil {
		t.Fatalf("skill not loaded; diagnostics: %+v", diags)
	}
	if s.Name != "my-skill" {
		t.Errorf("name = %q, want directory fallback my-skill", s.Name)
	}
	if s.Description != "A useful skill" {
		t.Errorf("description = %q", s.Description)
	}
	if s.Body != "# Instructions\n\nDo the thing." {
		t.Errorf("body = %q", s.Body)
	}
	if s.BaseDir != filepath.Dir(path) || s.Path != path {
		t.Errorf("path/baseDir = %q/%q", s.Path, s.BaseDir)
	}
	if len(diags) != 0 {
		t.Errorf("unexpected diagnostics: %+v", diags)
	}
}

func TestLoadSkillFile_ValidationWarnsButLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Bad_Name", SkillFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: Bad_Name\ndescription: " + strings.Repeat("x", maxDescriptionLength+1) +
		"\ndisable-model-invocation: true\n---\nBody"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	s, diags := loadSkillFile(path, ScopeUser)
	if s == nil {
		t.Fatal("invalid fields should warn, not drop the skill")
	}
	if !s.DisableModelInvocation {
		t.Error("disable-model-invocation not parsed")
	}
	var nameWarn, descWarn bool
	for _, d := range diags {
		if strings.Contains(d.Message, "name must use") {
			nameWarn = true
		}
		if strings.Contains(d.Message, "description exceeds") {
			descWarn = true
		}
	}
	if !nameWarn || !descWarn {
		t.Errorf("diagnostics = %+v, want name and description warnings", diags)
	}
}

func TestLoadSkillFile_Rejects(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "missing description", content: "---\nname: one\n---\nBody", want: "description is required"},
		{name: "missing frontmatter", content: "Body", want: "missing YAML frontmatter"},
		{name: "bad yaml", content: "---\nname: [\n---\nBody", want: "invalid frontmatter"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), SkillFileName)
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			s, diags := loadSkillFile(path, ScopeProject)
			if s != nil {
				t.Fatalf("skill = %+v, want nil", s)
			}
			if len(diags) != 1 || !strings.Contains(diags[0].Message, tc.want) {
				t.Fatalf("diagnostics = %+v, want contains %q", diags, tc.want)
			}
		})
	}
}
