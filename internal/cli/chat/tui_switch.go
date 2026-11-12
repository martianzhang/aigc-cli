package chat

import (
	"fmt"
	"sort"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/session"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// resumeProvider prefers the provider recorded by an earlier /provider or
// /model switch, but only when neither --provider nor defaults.chat.provider
// selected one explicitly.
func resumeProvider(cfg *types.ChatDefaults, sess *session.Session) *provider.EffectiveProvider {
	p := options.Shared.ResolveProvider(options.ProviderNameChat)
	if p == nil || sess == nil {
		return p
	}
	if options.Shared.ProviderSet || (cfg != nil && cfg.Provider != "") {
		return p
	}
	h := sess.Header()
	if h.Provider == "" {
		return p
	}
	if ep := options.Shared.ResolveNamedProvider(h.Provider); ep != nil {
		return ep
	}
	return p
}

// resumeModel prefers an explicit --model / defaults.chat.model (already
// merged into Shared.Model by runChatTUI), then the session's last model, then
// the selected provider's configured model.
func resumeModel(sess *session.Session, p *provider.EffectiveProvider) string {
	if options.Shared.Model != "" {
		return options.Shared.Model
	}
	if sess != nil {
		if sess.HasModelSelection() {
			return sess.Header().Model
		}
		if model := sess.Header().Model; model != "" {
			return model
		}
	}
	if p != nil {
		return p.Model
	}
	return ""
}

// applyModelSelection updates the live selection, recomputes the compaction
// window for the new provider/model, and records the choice in the session.
func (m *chatModel) applyModelSelection(providerName, model string) {
	settings := resolveCompactionSettingsWithDetect(m.cmd, options.ChatDefaults(), m.sess, providerName, model, false, false)

	m.mu.Lock()
	history := make([]types.ChatMessage, len(m.history))
	copy(history, m.history)
	m.mu.Unlock()

	m.provider = providerName
	m.model = model
	m.compactSettings = settings
	m.contextSize = settings.Window
	m.usageAnchor = nil
	m.contextTokens = agent.EstimateContext(history, nil, m.toolsTokens)
	m.compactCooldown = 0
	m.err = nil

	if m.sess != nil {
		source, size := "auto", 0
		if chatContextSize > 0 {
			source, size = "explicit", settings.Window
		}
		if err := m.sess.AppendModel(providerName, model, size, source); err != nil {
			m.messages = append(m.messages, message{role: "system", content: fmt.Sprintf("Session model update failed: %v", err)})
		}
	}
	m.input.Reset()
	m.refreshViewport()
}

func (m *chatModel) showModelSelection() {
	var b strings.Builder
	b.WriteString("Model selection\n")
	fmt.Fprintf(&b, "  current:  %s\n", m.modelDisplay())
	provider := m.provider
	if provider == "" {
		provider = "<default>"
	}
	fmt.Fprintf(&b, "  provider: %s\n", provider)
	if m.contextSize > 0 {
		fmt.Fprintf(&b, "  window:   %d tokens (estimated usage %d)\n", m.contextSize, m.contextTokens)
	} else {
		fmt.Fprintf(&b, "  window:   unknown (estimated usage %d)\n", m.contextTokens)
	}

	models, err := modelListForProvider(m.provider, modelContextWindowProbeTimeout)
	if err != nil {
		fmt.Fprintf(&b, "  models:   unavailable (%v)\n", err)
	} else {
		models = append([]types.OpenAIModel(nil), models...)
		sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
		b.WriteString("  available models:\n")
		defaultMarker := " "
		if m.model == "" {
			defaultMarker = "*"
		}
		fmt.Fprintf(&b, "    %s <API default>\n", defaultMarker)
		const maxModels = 80
		for i, model := range models {
			if i >= maxModels {
				fmt.Fprintf(&b, "    ... %d more\n", len(models)-maxModels)
				break
			}
			marker := " "
			if model.ID == m.model {
				marker = "*"
			}
			window := ""
			if w := model.ContextWindowTokens(); w > 0 {
				window = fmt.Sprintf("  (%d tokens)", w)
			}
			fmt.Fprintf(&b, "    %s %s%s\n", marker, model.ID, window)
		}
	}
	b.WriteString("  use /model <name> to switch within the current provider\n")
	b.WriteString("  use /model <provider> <name> to switch provider and model together\n")
	b.WriteString("  use /model default to let the API/provider choose\n")
	if names := options.ProviderNames(); len(names) > 0 {
		b.WriteString("  provider defaults:\n")
		for _, name := range names {
			np := options.NamedProvider(name)
			model := "<provider default>"
			if np != nil && np.Model != "" {
				model = np.Model
			}
			fmt.Fprintf(&b, "    %-18s %s\n", name, model)
		}
	}
	m.messages = append(m.messages, message{role: "system", content: strings.TrimRight(b.String(), "\n")})
	m.refreshViewport()
}

func (m *chatModel) switchModel(args string) {
	args = strings.TrimSpace(args)
	if args == "" {
		m.showModelSelection()
		return
	}
	// "/model <provider> <model>" switches both in one step. Model ids do not
	// contain whitespace, and using whitespace avoids the ambiguity of
	// OpenRouter-style "author/model" ids.
	if fields := strings.Fields(args); len(fields) >= 2 {
		providerName := fields[0]
		model := strings.Join(fields[1:], " ")
		if strings.EqualFold(model, "default") || strings.EqualFold(model, "auto") {
			model = ""
		}
		m.switchProviderWithModel(providerName, model, true)
		return
	}
	name := args
	if strings.EqualFold(name, "default") || strings.EqualFold(name, "auto") {
		name = ""
	}
	if name == m.model {
		m.messages = append(m.messages, message{role: "system", content: fmt.Sprintf("Already using model %s.", m.modelDisplay())})
		m.refreshViewport()
		return
	}
	old := m.modelDisplay()
	m.applyModelSelection(m.provider, name)
	m.messages = append(m.messages, message{
		role:    "system",
		content: fmt.Sprintf("Model switched: %s → %s.\nContext window is now %s.", old, m.modelDisplay(), m.contextWindowLabel()),
	})
	m.refreshViewport()
}

func (m *chatModel) showProviderSelection() {
	var b strings.Builder
	b.WriteString("Provider selection\n")
	current := m.provider
	if current == "" {
		current = "<default>"
	}
	fmt.Fprintf(&b, "  current: %s\n", current)
	names := options.ProviderNames()
	if len(names) == 0 {
		b.WriteString("  no named providers configured (add providers.<name> to config.yaml)\n")
	} else {
		b.WriteString("  configured:\n")
		for _, name := range names {
			np := options.NamedProvider(name)
			marker := " "
			if name == m.provider {
				marker = "*"
			}
			kind := "openai"
			model := "<provider default>"
			if np != nil {
				if np.Type != "" {
					kind = string(np.Type)
				}
				if np.Model != "" {
					model = np.Model
				}
				if np.ContextWindow > 0 {
					model = fmt.Sprintf("%s (%d)", model, np.ContextWindow)
				}
			}
			fmt.Fprintf(&b, "  %s %-18s %-10s %s\n", marker, name, kind, model)
		}
	}
	b.WriteString("  use /provider <name> to switch\n")
	b.WriteString("  use /provider default to restore the CLI/config provider\n")
	m.messages = append(m.messages, message{role: "system", content: strings.TrimRight(b.String(), "\n")})
	m.refreshViewport()
}

func (m *chatModel) switchProvider(name string) {
	m.switchProviderWithModel(name, "", false)
}

func (m *chatModel) switchProviderWithModel(name, modelOverride string, explicitModel bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		m.showProviderSelection()
		return
	}

	var p *provider.EffectiveProvider
	if strings.EqualFold(name, "default") || strings.EqualFold(name, "global") {
		p = options.Shared.ResolveProvider(options.ProviderNameChat)
	} else {
		p = options.Shared.ResolveNamedProvider(name)
		if p == nil {
			m.messages = append(m.messages, message{role: "system", content: fmt.Sprintf("Provider %q is not configured. Use /provider to list configured providers.", name)})
			m.refreshViewport()
			return
		}
	}
	if p == nil {
		m.messages = append(m.messages, message{role: "system", content: "No default provider could be resolved."})
		m.refreshViewport()
		return
	}
	if p.Type == types.ProviderLocal {
		m.messages = append(m.messages, message{role: "system", content: fmt.Sprintf("Provider %q is local and cannot serve chat completions.", providerDisplayName(p))})
		m.refreshViewport()
		return
	}
	if err := options.RequireBaseURL(options.ProviderNameChat, p); err != nil {
		m.messages = append(m.messages, message{role: "system", content: fmt.Sprintf("Cannot switch provider: %v", err)})
		m.refreshViewport()
		return
	}

	newProvider := providerDisplayName(p)
	model := p.Model
	switch {
	case explicitModel:
		model = modelOverride
	case newProvider == m.provider || model == "":
		model = m.model
	}
	if newProvider == m.provider && model == m.model {
		m.messages = append(m.messages, message{role: "system", content: fmt.Sprintf("Already using provider %s with model %s.", newProvider, m.modelDisplay())})
		m.refreshViewport()
		return
	}

	oldProvider := m.provider
	if oldProvider == "" {
		oldProvider = "<default>"
	}
	oldModel := m.model
	m.client = client.NewFromProvider(p)
	m.applyModelSelection(newProvider, model)
	notice := fmt.Sprintf("Provider switched: %s → %s.", oldProvider, newProvider)
	if model != oldModel {
		notice += fmt.Sprintf(" Model: %s.", m.modelDisplay())
	}
	notice += "\nContext window is now " + m.contextWindowLabel() + "."
	m.messages = append(m.messages, message{role: "system", content: notice})
	m.refreshViewport()
}

func (m *chatModel) contextWindowLabel() string {
	if m.contextSize > 0 {
		return fmt.Sprintf("%d tokens", m.contextSize)
	}
	return "unknown (auto-compaction off)"
}
