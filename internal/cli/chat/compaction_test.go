package chat

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/session"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func testCompactionCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "chat"}
	cmd.Flags().Int("context-size", 0, "")
	cmd.Flags().Int("reserve-tokens", 0, "")
	cmd.Flags().Int("keep-recent-tokens", 0, "")
	return cmd
}

func withChatConfig(t *testing.T, cfg *types.Config, contextSize, reserve, keep int) {
	t.Helper()
	oldCfg, oldContext, oldReserve, oldKeep := options.Shared.Cfg, chatContextSize, chatReserveTokens, chatKeepRecentTokens
	oldNoAuto, oldModel, oldMax, oldChars := chatNoAutoCompact, chatCompactModel, chatCompactMaxTokens, chatCompactToolResultChars
	oldDetector := detectModelContextWindowFn
	options.Shared.Cfg = cfg
	chatContextSize, chatReserveTokens, chatKeepRecentTokens = contextSize, reserve, keep
	chatNoAutoCompact, chatCompactModel, chatCompactMaxTokens, chatCompactToolResultChars = false, "", 0, 0
	// Keep resolver tests offline by default; probe-specific tests override.
	detectModelContextWindowFn = func(string) int { return 0 }
	t.Cleanup(func() {
		options.Shared.Cfg = oldCfg
		chatContextSize, chatReserveTokens, chatKeepRecentTokens = oldContext, oldReserve, oldKeep
		chatNoAutoCompact, chatCompactModel, chatCompactMaxTokens, chatCompactToolResultChars = oldNoAuto, oldModel, oldMax, oldChars
		detectModelContextWindowFn = oldDetector
	})
}

func TestResolveCompactionSettingsBuiltinWindow(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	got := resolveCompactionSettings(testCompactionCmd(), nil, nil, "", "claude-sonnet-4")
	if got.Window != 200000 {
		t.Fatalf("Window = %d, want 200000", got.Window)
	}
	if !got.Enabled {
		t.Fatal("Enabled = false, want true")
	}
}

func TestResolveCompactionSettingsExplicitWins(t *testing.T) {
	withChatConfig(t, nil, 32000, 4096, 8000)
	got := resolveCompactionSettings(testCompactionCmd(), nil, nil, "", "claude-sonnet-4")
	if got.Window != 32000 || got.ReserveTokens != 4096 || got.KeepRecentTokens != 8000 {
		t.Fatalf("settings = %+v", got)
	}
}

func TestResolveCompactionSettingsProviderAndOverride(t *testing.T) {
	cfg := &types.Config{
		Providers: map[string]*types.NamedProvider{
			"acme": {
				ContextWindow: 64000,
				Models: map[string]*types.ProviderModelConfig{
					"large": {ContextWindow: 128000},
				},
			},
		},
		Defaults: &types.ConfigDefaults{Chat: &types.ChatDefaults{
			ModelOverrides: map[string]*types.ChatCompactionOverride{
				"acme/large": {ContextSize: 96000, KeepRecentTokens: 12000},
			},
		}},
	}
	withChatConfig(t, cfg, 0, 0, 0)
	got := resolveCompactionSettings(testCompactionCmd(), cfg.Defaults.Chat, nil, "acme", "large")
	if got.Window != 96000 || got.KeepRecentTokens != 12000 {
		t.Fatalf("override settings = %+v", got)
	}

	got = resolveCompactionSettings(testCompactionCmd(), nil, nil, "acme", "large")
	if got.Window != 128000 {
		t.Fatalf("provider model window = %d, want 128000", got.Window)
	}
	got = resolveCompactionSettings(testCompactionCmd(), nil, nil, "acme", "small")
	if got.Window != 64000 {
		t.Fatalf("provider window = %d, want 64000", got.Window)
	}
}

func TestResolveCompactionSettingsSessionHeader(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	path := filepath.Join(t.TempDir(), "s.jsonl")
	sess, err := session.New(path, session.Header{
		ID: "s", Provider: "acme", Model: "claude-sonnet-4",
		ContextSize: 48000, ContextSizeSource: "explicit",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close("test")

	got := resolveCompactionSettings(testCompactionCmd(), nil, sess, "acme", "claude-sonnet-4")
	if got.Window != 48000 {
		t.Fatalf("same-model header window = %d, want 48000", got.Window)
	}
	// Switching models must not inherit the old model's explicit window.
	got = resolveCompactionSettings(testCompactionCmd(), nil, sess, "acme", "gpt-4o")
	if got.Window != 128000 {
		t.Fatalf("switched-model window = %d, want the new model's 128000", got.Window)
	}

	// An explicit --context-size 0 asks for auto-detection and must not
	// inherit the session's old explicit value.
	autoCmd := testCompactionCmd()
	if err := autoCmd.Flags().Set("context-size", "0"); err != nil {
		t.Fatal(err)
	}
	got = resolveCompactionSettings(autoCmd, nil, sess, "acme", "claude-sonnet-4")
	if got.Window != 200000 {
		t.Fatalf("explicit --context-size 0 window = %d, want auto-detected 200000", got.Window)
	}
}

func TestResolveCompactionSettingsAutoCompactOff(t *testing.T) {
	off := false
	cfg := &types.Config{Defaults: &types.ConfigDefaults{Chat: &types.ChatDefaults{AutoCompact: &off}}}
	withChatConfig(t, cfg, 128000, 0, 0)
	got := resolveCompactionSettings(testCompactionCmd(), cfg.Defaults.Chat, nil, "", "claude-sonnet-4")
	if got.Enabled {
		t.Fatal("Enabled = true, want false")
	}
}

func TestCurrentSessionHeaderContextSource(t *testing.T) {
	withChatConfig(t, nil, 32768, 0, 0)
	if h := currentSessionHeader(); h.ContextSize != 32768 || h.ContextSizeSource != "explicit" {
		t.Fatalf("header = %+v, want explicit 32768", h)
	}
	withChatConfig(t, nil, 0, 0, 0)
	if h := currentSessionHeader(); h.ContextSize != 0 || h.ContextSizeSource != "auto" {
		t.Fatalf("header = %+v, want auto", h)
	}
}

func TestResolveCompactionSettingsUsesModelProbe(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	old := detectModelContextWindowFn
	var calls int
	detectModelContextWindowFn = func(model string) int {
		calls++
		if model != "brand-new-model" {
			t.Fatalf("probe model = %q", model)
		}
		return 99999
	}
	t.Cleanup(func() { detectModelContextWindowFn = old })

	got := resolveCompactionSettings(testCompactionCmd(), nil, nil, "", "brand-new-model")
	if got.Window != 99999 {
		t.Fatalf("Window = %d, want probe result 99999", got.Window)
	}
	if calls != 1 {
		t.Fatalf("probe calls = %d, want 1", calls)
	}
}

func TestResolveCompactionSettingsSkipsModelProbe(t *testing.T) {
	old := detectModelContextWindowFn
	calls := 0
	detectModelContextWindowFn = func(string) int { calls++; return 99999 }
	t.Cleanup(func() { detectModelContextWindowFn = old })

	// Detection disabled by config.
	off := false
	cfg := &types.Config{Defaults: &types.ConfigDefaults{Chat: &types.ChatDefaults{
		AutoDetectContextWindow: &off,
	}}}
	withChatConfig(t, cfg, 0, 0, 0)
	if got := resolveCompactionSettings(testCompactionCmd(), cfg.Defaults.Chat, nil, "", "brand-new-model"); got.Window != 0 {
		t.Fatalf("Window = %d, want 0 when detection is off", got.Window)
	}
	if calls != 0 {
		t.Fatalf("probe calls = %d, want 0", calls)
	}

	// Auto-compaction disabled: no need to probe either.
	noAuto := false
	cfg2 := &types.Config{Defaults: &types.ConfigDefaults{Chat: &types.ChatDefaults{
		AutoCompact: &noAuto,
	}}}
	withChatConfig(t, cfg2, 0, 0, 0)
	detectModelContextWindowFn = func(string) int { calls++; return 99999 }
	if got := resolveCompactionSettings(testCompactionCmd(), cfg2.Defaults.Chat, nil, "", "brand-new-model"); got.Window != 0 {
		t.Fatalf("Window = %d, want 0 when auto-compaction is off", got.Window)
	}
	if calls != 0 {
		t.Fatalf("probe calls = %d, want 0 when auto-compaction is off", calls)
	}
}

func TestResolveCompactionSettingsUnknownModel(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	got := resolveCompactionSettings(testCompactionCmd(), nil, nil, "", "totally-unknown-model")
	if got.Window != 0 {
		t.Fatalf("Window = %d, want 0 for unknown model", got.Window)
	}
}
