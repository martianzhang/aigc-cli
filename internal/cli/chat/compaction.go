package chat

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/session"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// resolveCompactionSettings builds the effective compaction budget. Resolution
// order for the window: --context-size / defaults.chat.context_size (already in
// chatContextSize) > an explicit session header for the same model > per-model
// override > provider config > built-in model table > unknown (disabled).
func resolveCompactionSettings(cmd *cobra.Command, cfg *types.ChatDefaults, sess *session.Session, providerName, model string) agent.Settings {
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
	if s.Window <= 0 && s.Enabled && model != "" && autoDetectContextWindow(cfg) {
		s.Window = detectModelContextWindowFn(model)
	}

	if !flagReserve && s.ReserveTokens <= 0 && override != nil && override.ReserveTokens > 0 {
		s.ReserveTokens = override.ReserveTokens
	}
	if !flagKeep && s.KeepRecentTokens <= 0 && override != nil && override.KeepRecentTokens > 0 {
		s.KeepRecentTokens = override.KeepRecentTokens
	}

	if s.Enabled && s.Window <= 0 {
		fmt.Fprintf(options.Stderr(), "[context] no context window known for model %q; auto-compaction is off. Set --context-size or defaults.chat.context_size.\n", model)
	}
	return s
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
