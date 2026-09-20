package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
)

func TestKnowledgeBaseDir_IsExpandedUnderHome(t *testing.T) {
	dir := knowledgeBaseDir()
	if strings.Contains(dir, "~") {
		t.Errorf("knowledgeBaseDir() = %q must not contain a literal ~", dir)
	}
	if want := filepath.Join(options.ConfigDir(), "knowledge"); dir != want {
		t.Errorf("knowledgeBaseDir() = %q, want %q", dir, want)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return
	}
	if !strings.HasPrefix(dir, home+string(os.PathSeparator)) {
		t.Errorf("knowledgeBaseDir() = %q, want a path under home %q", dir, home)
	}
}

func TestKbDir_UsesSingleExpandedHelper(t *testing.T) {
	if got, want := kbDir(), knowledgeBaseDir(); got != want {
		t.Errorf("kbDir() = %q, want %q", got, want)
	}
}
