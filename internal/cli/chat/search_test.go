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
			Defaults: &types.ConfigDefaults{Search: &types.SearchDefaults{Provider: "shared"}},
		}
		chatSearchProvider = "flag"
		if got := resolveSearchVendor(); got != "flag" {
			t.Errorf("resolveSearchVendor() = %q, want flag", got)
		}
	})

	t.Run("shared default applies", func(t *testing.T) {
		restore()
		options.Shared.Cfg = &types.Config{
			Defaults: &types.ConfigDefaults{Search: &types.SearchDefaults{Provider: "shared"}},
		}
		if got := resolveSearchVendor(); got != "shared" {
			t.Errorf("resolveSearchVendor() = %q, want shared", got)
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
	if !strings.Contains(got, "defaults.search.provider") {
		t.Errorf("executeWebSearch(no provider) = %q, want the shared scope hint", got)
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
