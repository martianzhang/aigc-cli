package chat

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/session"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// resolveCompactionSettings builds the effective compaction budget. Resolution
// order for the window: --context-size / defaults.chat.context_size (already in
// chatContextSize) > an explicit session header for the same model > per-model
// override > provider config > built-in model table > /models probe >
// fallback_context_window (default 128K when the window is still unknown).
func resolveCompactionSettings(cmd *cobra.Command, cfg *types.ChatDefaults, sess *session.Session, providerName, model string) agent.Settings {
	return resolveCompactionSettingsWithDetect(cmd, cfg, sess, providerName, model, true, true)
}

// resolveCompactionSettingsWithDetect allows interactive switches to skip the
// /models probe. The probe is bounded but can still block the TUI for up to two
// seconds; a switch should never leave buffered keystrokes fighting the input
// reset. Startup and resume keep detection enabled.
func resolveCompactionSettingsWithDetect(cmd *cobra.Command, cfg *types.ChatDefaults, sess *session.Session, providerName, model string, allowDetect, warn bool) agent.Settings {
	s := agent.Settings{
		Enabled:          true,
		Window:           chatContextSize,
		ReserveTokens:    chatReserveTokens,
		KeepRecentTokens: chatKeepRecentTokens,
		MaxOutputTokens:  chatMaxTokens,
		SummaryModel:     chatCompactModel,
		SummaryMaxTokens: chatCompactMaxTokens,
		ToolResultChars:  chatCompactToolResultChars,
	}

	if chatNoAutoCompact || (cfg != nil && cfg.AutoCompact != nil && !*cfg.AutoCompact) {
		s.Enabled = false
	}

	override := findCompactionOverride(cfg, providerName, model)
	flagReserve := cmd != nil && cmd.Flags().Changed("reserve-tokens")
	flagKeep := cmd != nil && cmd.Flags().Changed("keep-recent-tokens")

	forceAuto := cmd != nil && cmd.Flags().Changed("context-size") && chatContextSize == 0
	if s.Window <= 0 && sess != nil && !forceAuto {
		h := sess.Header()
		if sameSessionModel(h, providerName, model) && h.ContextSize > 0 && h.ContextSizeSource != "auto" {
			s.Window = h.ContextSize
		}
	}
	if s.Window <= 0 && override != nil && override.ContextSize > 0 {
		s.Window = override.ContextSize
	}
	if s.Window <= 0 {
		s.Window = providerContextWindow(providerName, model)
	}
	if s.Window <= 0 {
		s.Window = provider.BuiltinContextWindow(model)
	}
	// Unknown model: fall back to the provider's /models catalogue. The probe
	// is cached, bounded to two seconds, and skipped when auto-compaction is
	// disabled or the user turned detection off.
	if s.Window <= 0 && s.Enabled && model != "" && allowDetect && autoDetectContextWindow(cfg) {
		s.Window = detectModelContextWindowFn(providerName, model)
	}
	// Last resort: a configurable conservative window. This keeps compression
	// enabled instead of silently disabling it when the model is unknown.
	usedFallback := false
	if s.Window <= 0 && s.Enabled {
		s.Window = fallbackContextWindow(cfg)
		usedFallback = true
	}

	if !flagReserve && s.ReserveTokens <= 0 && override != nil && override.ReserveTokens > 0 {
		s.ReserveTokens = override.ReserveTokens
	}
	if !flagKeep && s.KeepRecentTokens <= 0 && override != nil && override.KeepRecentTokens > 0 {
		s.KeepRecentTokens = override.KeepRecentTokens
	}

	if warn && usedFallback {
		fmt.Fprintf(options.Stderr(), "[context] no context window known for model %q; using fallback %d tokens. Set --context-size or defaults.chat.context_size to override.\n", model, s.Window)
	}
	return s
}

// DefaultFallbackContextWindow is used when no window can be resolved from the
// user, the session, the provider, the model table or /models.
const DefaultFallbackContextWindow = 128000

func fallbackContextWindow(cfg *types.ChatDefaults) int {
	if cfg != nil && cfg.FallbackContextWindow > 0 {
		return cfg.FallbackContextWindow
	}
	return DefaultFallbackContextWindow
}

// providerForName returns the named provider when providerName matches a
// configured key, otherwise the command's default provider. It is used for
// model-aware work (window probing) after an interactive provider switch.
func providerForName(providerName string) *provider.EffectiveProvider {
	if providerName != "" {
		if p := options.Shared.ResolveNamedProvider(providerName); p != nil {
			return p
		}
	}
	return options.Shared.ResolveProvider(options.ProviderNameChat)
}

// providerDisplayName returns the name shown in the status bar and stored in
// the session: the named provider key, else the detected provider type.
func providerDisplayName(p *provider.EffectiveProvider) string {
	if p == nil {
		return ""
	}
	if p.Name != "" {
		return p.Name
	}
	if s := p.ProviderType.String(); s != "" && s != "unknown" {
		return s
	}
	if p.Type != "" {
		return string(p.Type)
	}
	return ""
}

func findCompactionOverride(cfg *types.ChatDefaults, providerName, model string) *types.ChatCompactionOverride {
	if cfg == nil || len(cfg.ModelOverrides) == 0 || model == "" {
		return nil
	}
	if providerName != "" {
		if o := cfg.ModelOverrides[providerName+"/"+model]; o != nil {
			return o
		}
	}
	return cfg.ModelOverrides[model]
}

func sameSessionModel(h session.Header, providerName, model string) bool {
	if h.Model != model {
		return false
	}
	if h.Provider != "" && providerName != "" && h.Provider != providerName {
		return false
	}
	return true
}

// providerContextWindow reads providers.<name>.context_window and its per-model
// overrides. It only applies to named providers; the global api_key/base_url
// path has no provider entry, so defaults.chat.context_size is the place to set
// a window there.
// compactionOptions builds the agent options used by both loops.
func compactionOptions(settings agent.Settings, anchor *agent.UsageAnchor, toolsTokens int, sess *session.Session, model, reason string, force bool, stderr io.Writer) agent.CompactOptions {
	readFiles, modifiedFiles := previousCompactionFiles(sess)
	return agent.CompactOptions{
		Model:                 model,
		Settings:              settings,
		Anchor:                anchor,
		ToolsTokens:           toolsTokens,
		Reason:                reason,
		Force:                 force,
		Stderr:                stderr,
		PreviousReadFiles:     readFiles,
		PreviousModifiedFiles: modifiedFiles,
	}
}

func previousCompactionFiles(sess *session.Session) (readFiles, modifiedFiles []string) {
	if sess == nil {
		return nil, nil
	}
	if rec := sess.LastCompaction(); rec != nil {
		return rec.ReadFiles, rec.ModifiedFiles
	}
	return nil, nil
}

// persistCompaction writes the checkpoint and keeps the result's tail verbatim.
func persistCompaction(sess *session.Session, result *agent.CompactResult, model string) error {
	if sess == nil || result == nil {
		return nil
	}
	return sess.AppendCompactionKeepLast(result.Summary, result.KeepLast, session.CompactionMeta{
		TokensBefore:  result.TokensBefore,
		TokensAfter:   result.Tokens,
		Model:         model,
		SummaryModel:  result.SummaryModel,
		Reason:        result.Reason,
		ReadFiles:     result.ReadFiles,
		ModifiedFiles: result.ModifiedFiles,
		Usage:         result.Usage,
	})
}

func providerContextWindow(providerName, model string) int {
	np := options.NamedProvider(providerName)
	if np == nil {
		return 0
	}
	if model != "" {
		if m := np.Models[model]; m != nil && m.ContextWindow > 0 {
			return m.ContextWindow
		}
	}
	if np.ContextWindow > 0 {
		return np.ContextWindow
	}
	return 0
}
