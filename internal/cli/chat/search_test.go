package chat

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// The chat search primitives are thin wrappers over internal/webtool (which owns
// the fetch/search behaviour and its tests). What remains chat-specific is vendor
// resolution from CLI flags + config scope.

func TestResolveSearchVendor_precedence(t *testing.T) {
	restore := func() {
		options.Shared.Cfg = nil
		chatSearchProvider = ""
	}
	t.Cleanup(restore)

	t.Run("flag wins", func(t *testing.T) {
		restore()
		options.Shared.Cfg = &types.Config{
			SearchProvider: "global",
			Defaults:       &types.ConfigDefaults{Chat: &types.ChatDefaults{SearchProvider: "chat"}},
		}
		chatSearchProvider = "flag"
		if got := resolveSearchVendor(); got != "flag" {
			t.Errorf("resolveSearchVendor() = %q, want flag", got)
		}
	})

	t.Run("chat scope overrides global", func(t *testing.T) {
		restore()
		options.Shared.Cfg = &types.Config{
			SearchProvider: "global",
			Defaults:       &types.ConfigDefaults{Chat: &types.ChatDefaults{SearchProvider: "chat"}},
		}
		if got := resolveSearchVendor(); got != "chat" {
			t.Errorf("resolveSearchVendor() = %q, want chat", got)
		}
	})

	t.Run("global default applies when no chat scope", func(t *testing.T) {
		restore()
		options.Shared.Cfg = &types.Config{SearchProvider: "global"}
		if got := resolveSearchVendor(); got != "global" {
			t.Errorf("resolveSearchVendor() = %q, want global", got)
		}
	})

	t.Run("auto when nothing configured", func(t *testing.T) {
		restore()
		if got := resolveSearchVendor(); got != "auto" {
			t.Errorf("resolveSearchVendor() = %q, want auto", got)
		}
	})
}

// The third-party primitives must point the user at the right config knob when
// no provider is configured, instead of silently using the built-in engine.
func TestExecuteWebSearch_noRemoteProvider(t *testing.T) {
	prev := options.Shared.Cfg
	options.Shared.Cfg = nil
	prevProvider := chatSearchProvider
	chatSearchProvider = ""
	t.Cleanup(func() { options.Shared.Cfg = prev; chatSearchProvider = prevProvider })

	got := executeWebSearch(`{"query":"hello"}`)
	if !strings.Contains(got, "third-party provider") {
		t.Errorf("executeWebSearch(no provider) = %q, want a hint about configuring one", got)
	}
	if !strings.Contains(got, "defaults.chat.search_provider") {
		t.Errorf("executeWebSearch(no provider) = %q, want the chat scope hint", got)
	}
}

func TestExecuteWebFetch_noRemoteProvider(t *testing.T) {
	prev := options.Shared.Cfg
	options.Shared.Cfg = nil
	prevProvider := chatSearchProvider
	chatSearchProvider = ""
	t.Cleanup(func() { options.Shared.Cfg = prev; chatSearchProvider = prevProvider })

	got := executeWebFetch(`{"url":"https://example.com/a"}`)
	if !strings.Contains(got, "no third-party fetch provider") {
		t.Errorf("executeWebFetch(no provider) = %q, want 'no third-party fetch provider'", got)
	}
}

func TestBuildAgentTools_webSearchPrivileged(t *testing.T) {
	prev := options.Shared.Cfg
	options.Shared.Cfg = nil
	t.Cleanup(func() { options.Shared.Cfg = prev })

	if toolNames(buildAgentTools(&types.ChatDefaults{}))["web_search"] {
		t.Error("web_search should be off by default")
	}

	options.Shared.Cfg = &types.Config{ToolsEnable: []string{"web_search"}}
	if !toolNames(buildAgentTools(&types.ChatDefaults{}))["web_search"] {
		t.Error("web_search should be enabled when listed in tools_enable")
	}
}
