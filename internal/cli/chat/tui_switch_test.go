package chat

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/martianzhang/aigc-cli/internal/cliutil"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/session"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func switchTestModel() *chatModel {
	m := newChatModel(nil, nil, 10, "old-model", "", nil, false, 0, 0, 0)
	m.provider = "old-provider"
	m.history = []types.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hello"},
	}
	m.contextSize = 100000
	return m
}

// closeTestSession releases the session a test attached to the model. Windows
// cannot remove a file that is still open, so every test that saves or switches
// sessions must close before t.TempDir cleanup runs.
func closeTestSession(m *chatModel) {
	if m != nil && m.sess != nil {
		_ = m.sess.Close("test cleanup")
	}
}

func newSwitchSession(t *testing.T, header session.Header) *session.Session {
	t.Helper()
	if header.ID == "" {
		header.ID = "switch"
	}
	s, err := session.New(filepath.Join(t.TempDir(), header.ID+".jsonl"), header)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close("test") })
	return s
}

func TestSwitchModelUpdatesSelection(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	m := switchTestModel()
	defer closeTestSession(m)
	m.input.SetValue("/model gpt-4o")
	m.switchModel("gpt-4o")
	if got := m.input.Value(); got != "" {
		t.Fatalf("switchModel left input behind: %q", got)
	}

	if m.model != "gpt-4o" {
		t.Fatalf("model = %q", m.model)
	}
	if m.provider != "old-provider" {
		t.Fatalf("provider changed to %q", m.provider)
	}
	if m.contextSize != 128000 {
		t.Fatalf("contextSize = %d, want 128000", m.contextSize)
	}
	if m.contextTokens == 0 {
		t.Fatal("contextTokens was not recomputed")
	}
	if got := m.messages[len(m.messages)-1].content; !strings.Contains(got, "Model switched") {
		t.Fatalf("notice = %q", got)
	}
	m.showModelSelection()
	if got := m.messages[len(m.messages)-1].content; !strings.Contains(got, "Model selection") || !strings.Contains(got, "gpt-4o") {
		t.Fatalf("model list = %q", got)
	}
}

func TestSwitchModelDefaultAndSame(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	m := switchTestModel()
	defer closeTestSession(m)
	m.switchModel("default")
	if m.model != "" {
		t.Fatalf("model = %q, want API default", m.model)
	}
	if got := m.messages[len(m.messages)-1].content; !strings.Contains(got, "<API default>") {
		t.Fatalf("notice = %q", got)
	}

	m.model = "gpt-4o"
	before := len(m.messages)
	m.switchModel("gpt-4o")
	if len(m.messages) != before+1 || !strings.Contains(m.messages[len(m.messages)-1].content, "Already using") {
		t.Fatalf("same-model notice = %+v", m.messages[before:])
	}
}

func TestHandleSubmitModelAndProviderCommands(t *testing.T) {
	cfg := &types.Config{Providers: map[string]*types.NamedProvider{
		"alt": {Type: types.ProviderOpenAI, BaseURL: "https://alt.example/v1", Model: "alt-model"},
	}}
	withChatConfig(t, cfg, 0, 0, 0)

	m := switchTestModel()
	defer closeTestSession(m)
	m.input.SetValue("/model gpt-4o")
	_, _ = m.handleSubmitInput()
	if m.model != "gpt-4o" {
		t.Fatalf("/model did not switch: %q", m.model)
	}
	if got := m.input.Value(); got != "" {
		t.Fatalf("/model command left input behind: %q", got)
	}

	m.input.SetValue("/provider alt")
	_, _ = m.handleSubmitInput()
	if m.provider != "alt" || m.model != "alt-model" {
		t.Fatalf("/provider did not switch: %q / %q", m.provider, m.model)
	}
	if got := m.input.Value(); got != "" {
		t.Fatalf("/provider command left input behind: %q", got)
	}
}

func TestSwitchModelPersistsSession(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	sess := newSwitchSession(t, session.Header{ID: "switch-persist", Model: "old-model"})
	m := switchTestModel()
	defer closeTestSession(m)
	m.sess = sess
	m.switchModel("gpt-4o")

	h := sess.Header()
	if h.Model != "gpt-4o" || h.Provider != "old-provider" || h.ContextSizeSource != "auto" {
		t.Fatalf("session header = %+v", h)
	}
	if h.ContextSize != 0 {
		t.Fatalf("auto context size = %d, want 0", h.ContextSize)
	}
}

func TestSwitchProviderNamed(t *testing.T) {
	cfg := &types.Config{Providers: map[string]*types.NamedProvider{
		"alt": {Type: types.ProviderOpenAI, BaseURL: "https://alt.example/v1", Model: "alt-model"},
	}}
	withChatConfig(t, cfg, 0, 0, 0)
	m := switchTestModel()
	defer closeTestSession(m)
	m.switchProvider("alt")

	if m.provider != "alt" || m.model != "alt-model" {
		t.Fatalf("selection = %q / %q", m.provider, m.model)
	}
	if m.client == nil || m.client.BaseURL() != "https://alt.example/v1" {
		t.Fatalf("client base URL = %v", m.client)
	}
	if got := m.messages[len(m.messages)-1].content; !strings.Contains(got, "Provider switched") {
		t.Fatalf("notice = %q", got)
	}
	m.showProviderSelection()
	if got := m.messages[len(m.messages)-1].content; !strings.Contains(got, "alt") || !strings.Contains(got, "alt-model") {
		t.Fatalf("provider list = %q", got)
	}
}

func TestSwitchModelDoesNotBlockOnModelProbe(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	calls := 0
	detectModelContextWindowFn = func(string, string) int { calls++; return 99999 }
	m := switchTestModel()
	defer closeTestSession(m)
	m.switchModel("brand-new-model")
	if calls != 0 {
		t.Fatalf("interactive switch ran the /models probe %d times", calls)
	}
}

func TestSwitchModelDoesNotWriteToStderr(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	var stderr bytes.Buffer
	restore := cliutil.SetStderr(&stderr)
	t.Cleanup(restore)

	m := switchTestModel()
	defer closeTestSession(m)
	m.switchModel("definitely-not-a-real-model")
	if stderr.Len() != 0 {
		t.Fatalf("interactive model switch wrote raw stderr: %q", stderr.String())
	}
	if m.contextSize != DefaultFallbackContextWindow {
		t.Fatalf("contextSize = %d, want fallback %d", m.contextSize, DefaultFallbackContextWindow)
	}
}

func TestSwitchModelWithProviderArgument(t *testing.T) {
	cfg := &types.Config{Providers: map[string]*types.NamedProvider{
		"alt": {Type: types.ProviderOpenAI, BaseURL: "https://alt.example/v1", Model: "alt-model"},
	}}
	withChatConfig(t, cfg, 0, 0, 0)
	m := switchTestModel()
	defer closeTestSession(m)
	m.switchModel("alt gpt-4o")
	if m.provider != "alt" || m.model != "gpt-4o" {
		t.Fatalf("selection = %q / %q", m.provider, m.model)
	}
	if m.client == nil || m.client.BaseURL() != "https://alt.example/v1" {
		t.Fatalf("client base URL = %v", m.client)
	}
}

func TestShowModelSelectionListsProviderModels(t *testing.T) {
	resetModelContextCache()
	t.Cleanup(resetModelContextCache)
	srv, _ := modelListServer(t, `[
		{"id":"gpt-4o","context_length":128000},
		{"id":"claude-sonnet-4","context_length":200000}
	]`, 200)
	defer srv.Close()
	cfg := &types.Config{Providers: map[string]*types.NamedProvider{
		"alt": {Type: types.ProviderOpenAI, BaseURL: srv.URL},
	}}
	withChatConfig(t, cfg, 0, 0, 0)
	m := switchTestModel()
	defer closeTestSession(m)
	m.provider = "alt"
	m.model = "gpt-4o"
	m.showModelSelection()
	text := m.messages[len(m.messages)-1].content
	for _, want := range []string{"available models", "gpt-4o", "claude-sonnet-4", "128000", "200000"} {
		if !strings.Contains(text, want) {
			t.Fatalf("model list missing %q:\n%s", want, text)
		}
	}
}

func TestModelAndProviderTabCompletion(t *testing.T) {
	resetModelContextCache()
	t.Cleanup(resetModelContextCache)
	srv, _ := modelListServer(t, `[
		{"id":"gpt-4o","context_length":128000},
		{"id":"claude-sonnet-4","context_length":200000}
	]`, 200)
	defer srv.Close()
	cfg := &types.Config{Providers: map[string]*types.NamedProvider{
		"alt": {Type: types.ProviderOpenAI, BaseURL: srv.URL},
	}}
	withChatConfig(t, cfg, 0, 0, 0)
	m := switchTestModel()
	defer closeTestSession(m)
	m.provider = "alt"

	m.input.SetValue("/provider al")
	_, _ = m.handleTabCompletion()
	if got := m.input.Value(); got != "/provider alt" {
		t.Fatalf("provider completion = %q", got)
	}

	m.cycleMatches, m.cycleIdx = nil, -1
	m.input.SetValue("/model alt gp")
	_, _ = m.handleTabCompletion()
	if got := m.input.Value(); got != "/model alt gpt-4o" {
		t.Fatalf("provider+model completion = %q", got)
	}

	m.cycleMatches, m.cycleIdx = nil, -1
	m.input.SetValue("/model cl")
	_, _ = m.handleTabCompletion()
	if got := m.input.Value(); got != "/model claude-sonnet-4" {
		t.Fatalf("model completion = %q", got)
	}
}

func TestProviderThenModelCommandClearsInput(t *testing.T) {
	cfg := &types.Config{Providers: map[string]*types.NamedProvider{
		"alt": {Type: types.ProviderOpenAI, BaseURL: "https://alt.example/v1", Model: "alt-model"},
	}}
	withChatConfig(t, cfg, 0, 0, 0)
	m := switchTestModel()
	defer closeTestSession(m)
	m.width, m.height = 100, 30

	// The reported sequence: switch provider, then switch model. Both commands
	// must leave the input empty and the box at its one-row height.
	m.input.SetValue("/provider alt")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*chatModel)
	if got := m.input.Value(); got != "" {
		t.Fatalf("provider command left input behind: %q", got)
	}

	m.input.SetValue("/model gpt-4o")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*chatModel)
	if got := m.input.Value(); got != "" {
		t.Fatalf("model command left input behind: %q", got)
	}
	if m.inputRows != 1 {
		t.Fatalf("inputRows = %d after model switch, want 1", m.inputRows)
	}
	if transcript := m.renderMessages(); strings.Contains(transcript, "/model") {
		t.Fatalf("model command leaked into the transcript:\n%s", transcript)
	}
}

func TestSwitchProviderKeepsModelWithoutProviderDefault(t *testing.T) {
	cfg := &types.Config{Providers: map[string]*types.NamedProvider{
		"alt": {Type: types.ProviderOpenAI, BaseURL: "https://alt.example/v1"},
	}}
	withChatConfig(t, cfg, 0, 0, 0)
	m := switchTestModel()
	defer closeTestSession(m)
	m.model = "keep-me"
	m.switchProvider("alt")
	if m.model != "keep-me" {
		t.Fatalf("model = %q, want keep-me", m.model)
	}
}

func TestSwitchProviderUnknownAndLocal(t *testing.T) {
	cfg := &types.Config{Providers: map[string]*types.NamedProvider{
		"local": {Type: types.ProviderLocal},
	}}
	withChatConfig(t, cfg, 0, 0, 0)
	m := switchTestModel()
	defer closeTestSession(m)

	m.switchProvider("missing")
	if got := m.messages[len(m.messages)-1].content; !strings.Contains(got, "not configured") {
		t.Fatalf("missing provider notice = %q", got)
	}
	m.switchProvider("local")
	if got := m.messages[len(m.messages)-1].content; !strings.Contains(got, "local") {
		t.Fatalf("local provider notice = %q", got)
	}
}

func TestResumeProviderUsesSessionSelection(t *testing.T) {
	cfg := &types.Config{Providers: map[string]*types.NamedProvider{
		"alt":   {Type: types.ProviderOpenAI, BaseURL: "https://alt.example/v1"},
		"other": {Type: types.ProviderOpenAI, BaseURL: "https://other.example/v1"},
	}}
	withChatConfig(t, cfg, 0, 0, 0)
	prevSet, prevProvider := options.Shared.ProviderSet, options.Shared.Provider
	options.Shared.ProviderSet, options.Shared.Provider = false, ""
	t.Cleanup(func() {
		options.Shared.ProviderSet, options.Shared.Provider = prevSet, prevProvider
	})

	sess := newSwitchSession(t, session.Header{ID: "resume-provider", Provider: "alt", Model: "m"})
	p := resumeProvider(nil, sess)
	if p == nil || p.Name != "alt" {
		t.Fatalf("resumeProvider() = %+v, want alt", p)
	}

	cfg.Defaults = &types.ConfigDefaults{Chat: &types.ChatDefaults{Provider: "other"}}
	p = resumeProvider(cfg.Defaults.Chat, sess)
	if p == nil || p.Name != "other" {
		t.Fatalf("resumeProvider() with config default = %+v, want other", p)
	}
}

func TestResumeModelPrecedence(t *testing.T) {
	prevModel := options.Shared.Model
	t.Cleanup(func() { options.Shared.Model = prevModel })
	options.Shared.Model = ""

	sess := newSwitchSession(t, session.Header{ID: "resume-model", Model: "session-model"})
	p := &provider.EffectiveProvider{Model: "provider-model"}
	if got := resumeModel(sess, p); got != "session-model" {
		t.Fatalf("resumeModel() = %q, want session-model", got)
	}
	options.Shared.Model = "flag-model"
	if got := resumeModel(sess, p); got != "flag-model" {
		t.Fatalf("resumeModel() = %q, want flag-model", got)
	}
	options.Shared.Model = ""
	if got := resumeModel(nil, p); got != "provider-model" {
		t.Fatalf("resumeModel() without session = %q, want provider-model", got)
	}

	// An explicit /model default is recorded as an empty model and must stay
	// empty rather than falling back to the provider's configured model.
	defaultSess := newSwitchSession(t, session.Header{ID: "resume-default"})
	if err := defaultSess.AppendModel("", "", 0, "auto"); err != nil {
		t.Fatal(err)
	}
	if got := resumeModel(defaultSess, p); got != "" {
		t.Fatalf("resumeModel() after /model default = %q, want empty", got)
	}
}
