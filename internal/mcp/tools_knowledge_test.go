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

func TestResolveKBSearchProvider(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *Config
		requested string
		want      string
	}{
		{"per-call provider wins", &Config{SearchProvider: "brave"}, "exa", "exa"},
		{"config default fallback", &Config{SearchProvider: "brave"}, "", "brave"},
		{"duckduckgo fallback", &Config{}, "", "duckduckgo"},
		{"nil config", nil, "", "duckduckgo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveKBSearchProvider(tt.cfg, tt.requested); got != tt.want {
				t.Errorf("resolveKBSearchProvider() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestKbSearchURLs_ForwardsRequestProvider proves the tool's per-call
// `provider` argument reaches the router. An unknown name must fail with that
// name in the error instead of silently searching with the configured default.
func TestKbSearchURLs_ForwardsRequestProvider(t *testing.T) {
	_, err := kbSearchURLs(&Config{}, "query", "definitely-not-a-provider", 1)
	if err == nil {
		t.Fatal("expected an error for an unknown search provider")
	}
	if !strings.Contains(err.Error(), "definitely-not-a-provider") {
		t.Errorf("error = %v, want it to name the requested provider", err)
	}
}
