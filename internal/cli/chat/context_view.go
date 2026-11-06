package chat

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// contextPercent returns the cached context usage as a percentage, or -1 when
// the window is unknown.
func (m *chatModel) contextPercent() float64 {
	if m.contextSize <= 0 || m.contextTokens <= 0 {
		return -1
	}
	pct := float64(m.contextTokens) * 100 / float64(m.contextSize)
	if pct > 999 {
		pct = 999
	}
	return pct
}

// formatContextPercent renders a percentage with up to two decimals, dropping
// trailing zeros so whole values stay compact ("40" instead of "40.00"). Tiny
// non-zero values are shown as "<0.01" rather than a misleading "0".
func formatContextPercent(pct float64) string {
	if pct <= 0 {
		return "0"
	}
	s := strconv.FormatFloat(pct, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "0" {
		return "<0.01"
	}
	return s
}

func (m *chatModel) showContext() {
	m.mu.Lock()
	history := make([]types.ChatMessage, len(m.history))
	copy(history, m.history)
	m.mu.Unlock()

	usage := agent.AnalyzeContext(history, m.usageAnchor, m.toolsTokens)
	// The cached value was computed by the agent loop with the latest usage
	// anchor; prefer it over a pure estimate for the headline number.
	if m.contextTokens > 0 {
		usage.Tokens = m.contextTokens
	}
	settings := m.compactSettings.Normalize()

	var b strings.Builder
	b.WriteString("Context usage\n")
	if settings.Window > 0 {
		pct := float64(usage.Tokens) * 100 / float64(settings.Window)
		fmt.Fprintf(&b, "  %d / %d tokens (%s%%)\n", usage.Tokens, settings.Window, formatContextPercent(pct))
		fmt.Fprintf(&b, "  compact at %d (reserve %d, keep recent %d)\n",
			settings.Threshold(), settings.ReserveTokens, settings.KeepRecentTokens)
		if !settings.Enabled {
			b.WriteString("  auto-compaction is OFF (manual /compact still works)\n")
		}
	} else {
		fmt.Fprintf(&b, "  %d tokens (window unknown; set --context-size or defaults.chat.context_size)\n", usage.Tokens)
	}

	br := usage.Breakdown
	fmt.Fprintf(&b, "  system %d | tools %d | user %d | assistant %d | tool results %d | images %d (subset)\n",
		br.System, br.Tools, br.User, br.Assistant, br.ToolResults, br.Images)
	if usage.AnchorUsed {
		fmt.Fprintf(&b, "  usage anchor: %d prompt tokens + %d estimated tail\n", usage.UsagePart, usage.TailPart)
	} else {
		b.WriteString("  usage anchor: none (all estimated)\n")
	}

	if last := m.lastCompaction; last != nil {
		fmt.Fprintf(&b, "  last compaction: reason=%s, %d -> %d (saved %d)\n",
			last.Reason, last.TokensBefore, last.Tokens, last.TokenSaved)
		summaryModel := last.SummaryModel
		if summaryModel == "" {
			summaryModel = "default"
		}
		fmt.Fprintf(&b, "  summary %d tokens via %s, kept last %d messages\n",
			agent.EstimateTokens(last.Summary), summaryModel, last.KeepLast)
		if last.Usage != nil {
			fmt.Fprintf(&b, "  summary usage: %d in / %d out", last.Usage.PromptTokens, last.Usage.CompletionTokens)
			if last.Usage.Cost > 0 {
				fmt.Fprintf(&b, ", $%.6f", last.Usage.Cost)
			}
			b.WriteString("\n")
		}
	}
	if m.compactCooldown > 0 {
		fmt.Fprintf(&b, "  compaction cooldown: %d turns after a failure\n", m.compactCooldown)
	}

	m.messages = append(m.messages, message{role: "system", content: strings.TrimRight(b.String(), "\n")})
	m.refreshViewport()
}
