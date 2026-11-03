package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	maxNameLength        = 64
	maxDescriptionLength = 1024
)

// skillNameRE implements the Agent Skills name rules: lowercase letters,
// numbers and hyphens, with no leading, trailing, or consecutive hyphens.
var skillNameRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type skillFrontmatter struct {
	Name                   string `yaml:"name"`
	Description            string `yaml:"description"`
	DisableModelInvocation bool   `yaml:"disable-model-invocation"`
}

// loadSkillFile parses one SKILL.md. It returns nil when the file is not a
// usable skill (unreadable, malformed frontmatter, or missing description).
func loadSkillFile(path string, scope Scope) (*Skill, []Diagnostic) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, []Diagnostic{{Path: path, Message: fmt.Sprintf("cannot read skill: %v", err)}}
	}

	frontmatter, body, err := splitFrontmatter(string(raw))
	if err != nil {
		return nil, []Diagnostic{{Path: path, Message: err.Error()}}
	}

	var meta skillFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatter), &meta); err != nil {
		return nil, []Diagnostic{{Path: path, Message: fmt.Sprintf("invalid frontmatter: %v", err)}}
	}

	description := strings.TrimSpace(meta.Description)
	if description == "" {
		return nil, []Diagnostic{{Path: path, Message: "description is required"}}
	}

	name := strings.TrimSpace(meta.Name)
	if name == "" {
		name = filepath.Base(filepath.Dir(path))
	}

	var diags []Diagnostic
	for _, message := range validateName(name) {
		diags = append(diags, Diagnostic{Path: path, Message: message})
	}
	for _, message := range validateDescription(description) {
		diags = append(diags, Diagnostic{Path: path, Message: message})
	}

	return &Skill{
		Name:                   name,
		Description:            description,
		Path:                   path,
		BaseDir:                filepath.Dir(path),
		Scope:                  scope,
		DisableModelInvocation: meta.DisableModelInvocation,
		Body:                   body,
	}, diags
}

// splitFrontmatter separates the YAML block from the markdown body. SKILL.md
// must start with a --- delimiter; a BOM and CRLF line endings are accepted.
func splitFrontmatter(content string) (frontmatter, body string, err error) {
	content = strings.TrimPrefix(content, "\ufeff")
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", fmt.Errorf("missing YAML frontmatter")
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "---" {
			continue
		}
		return strings.Join(lines[1:i], "\n"),
			strings.TrimSpace(strings.Join(lines[i+1:], "\n")),
			nil
	}
	return "", "", fmt.Errorf("unterminated YAML frontmatter")
}

func validateName(name string) []string {
	var messages []string
	if utf8.RuneCountInString(name) > maxNameLength {
		messages = append(messages,
			fmt.Sprintf("name exceeds %d characters", maxNameLength))
	}
	if !skillNameRE.MatchString(name) {
		messages = append(messages,
			"name must use lowercase letters, numbers, and single hyphens (no leading, trailing, or consecutive hyphens)")
	}
	return messages
}

func validateDescription(description string) []string {
	if utf8.RuneCountInString(description) > maxDescriptionLength {
		return []string{fmt.Sprintf("description exceeds %d characters", maxDescriptionLength)}
	}
	return nil
}
