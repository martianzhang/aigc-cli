package knowledge

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestExtractHost(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"host only", "https://example.com/a/b", "example.com"},
		{"host with port", "https://example.com:8080/x", "example.com:8080"},
		{"invalid", "://bad", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractHost(tc.in); got != tc.want {
				t.Errorf("extractHost(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestSubcommandsRegisteredOnce guards against duplicate cobra registration:
// every subcommand name must appear exactly once under its parent. Duplicates
// make `knowledgebase --help` list the same command twice.
func TestSubcommandsRegisteredOnce(t *testing.T) {
	// Given the fully-initialized knowledge base command tree
	root := Cmd()

	// When/Then each parent registers every direct subcommand exactly once
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		counts := make(map[string]int)
		for _, sub := range cmd.Commands() {
			counts[sub.Name()]++
		}
		for name, n := range counts {
			if n != 1 {
				t.Errorf("%s: subcommand %q registered %d times, want exactly 1", cmd.CommandPath(), name, n)
			}
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(root)

	// And the expected top-level subcommands are all present
	want := []string{
		"add", "fetch", "find", "index", "init", "list", "map",
		"prune", "reset", "rm", "search", "show", "vault",
	}
	got := make(map[string]bool)
	for _, sub := range root.Commands() {
		got[sub.Name()] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("%s: subcommand %q is not registered", root.CommandPath(), name)
		}
	}
}

func TestResolveSearchStrategy(t *testing.T) {
	tests := []struct {
		provider   string
		wantStrat  string
		wantManual []string
	}{
		{"auto", "auto", nil},
		{"", "auto", nil},
		{"free", "cheap", nil},
		{"cheap", "cheap", nil},
		{"quality", "quality", nil},
		{"firecrawl", "manual", []string{"firecrawl"}},
	}
	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			strat, manual := resolveSearchStrategy(tc.provider)
			if strat != tc.wantStrat {
				t.Errorf("strategy = %q, want %q", strat, tc.wantStrat)
			}
			if len(manual) != len(tc.wantManual) {
				t.Fatalf("manual = %v, want %v", manual, tc.wantManual)
			}
			for i := range manual {
				if manual[i] != tc.wantManual[i] {
					t.Fatalf("manual = %v, want %v", manual, tc.wantManual)
				}
			}
		})
	}
}
