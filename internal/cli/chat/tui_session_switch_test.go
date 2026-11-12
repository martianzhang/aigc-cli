package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/session"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// withSessionGlobals isolates the process-wide chat selection so a switch test
// does not inherit --model/--provider state from another test.
func withSessionGlobals(t *testing.T) {
	t.Helper()
	oldModel, oldProvider, oldSet := options.Shared.Model, options.Shared.Provider, options.Shared.ProviderSet
	options.Shared.Model, options.Shared.Provider, options.Shared.ProviderSet = "", "", false
	t.Cleanup(func() {
		options.Shared.Model, options.Shared.Provider, options.Shared.ProviderSet = oldModel, oldProvider, oldSet
	})
}

func createSessionFile(t *testing.T, dir, id, name, sys, model string, msgs ...types.ChatMessage) string {
	t.Helper()
	path := filepath.Join(dir, id+".jsonl")
	s, err := session.New(path, session.Header{ID: id, Model: model})
	if err != nil {
		t.Fatalf("New(%s) error = %v", id, err)
	}
	if sys != "" {
		if err := s.AppendSystem(sys); err != nil {
			t.Fatal(err)
		}
	}
	for _, msg := range msgs {
		if err := s.AppendMessage(msg, "", 0); err != nil {
			t.Fatal(err)
		}
	}
	if name != "" {
		if err := s.AppendName(name); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close("test"); err != nil {
		t.Fatal(err)
	}
	return path
}

func openSession(t *testing.T, path string) *session.Session {
	t.Helper()
	s, err := session.Open(path)
	if err != nil {
		t.Fatalf("Open(%s) error = %v", path, err)
	}
	t.Cleanup(func() { _ = s.Close("test cleanup") })
	return s
}

func TestShowSessionListMarksCurrentAndListsRecent(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	withSessionGlobals(t)

	dir := t.TempDir()
	now := time.Now()
	alpha := createSessionFile(t, dir, "alpha", "Alpha work", "", "", types.ChatMessage{Role: "user", Content: "alpha question"})
	beta := createSessionFile(t, dir, "beta", "", "", "gpt-test", types.ChatMessage{Role: "user", Content: "beta question"})
	if err := os.Chtimes(alpha, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(beta, now, now); err != nil {
		t.Fatal(err)
	}

	m := switchTestModel()
	defer closeTestSession(m)
	m.sessionDirPath = dir
	m.sess = openSession(t, beta)
	m.showSessionList()

	content := m.messages[len(m.messages)-1].content
	for _, want := range []string{"Sessions in", "Alpha work", "beta", "gpt-test", "alpha question"} {
		if !strings.Contains(content, want) {
			t.Errorf("session list missing %q:\n%s", want, content)
		}
	}
	if !strings.Contains(content, "*  1") {
		t.Errorf("current session is not marked:\n%s", content)
	}
	if strings.Index(content, "beta question") > strings.Index(content, "alpha question") {
		t.Errorf("sessions are not newest-first:\n%s", content)
	}
}

func TestShowSessionListEmpty(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	withSessionGlobals(t)

	m := switchTestModel()
	defer closeTestSession(m)
	m.sessionDirPath = t.TempDir()
	m.showSessionList()
	if got := m.messages[len(m.messages)-1].content; !strings.Contains(got, "No saved sessions") {
		t.Fatalf("empty list message = %q", got)
	}
}

func TestPerformSessionSwitchAppliesState(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	withSessionGlobals(t)

	dir := t.TempDir()
	createSessionFile(t, dir, "current", "", "", "current-model", types.ChatMessage{Role: "user", Content: "old chat"})
	targetPath := createSessionFile(t, dir, "target", "Target work", "stored system prompt", "target-model",
		types.ChatMessage{Role: "user", Content: "hello"},
		types.ChatMessage{Role: "assistant", Content: "hi"},
	)

	m := switchTestModel()
	defer closeTestSession(m)
	m.sessionDirPath = dir
	m.sess = openSession(t, filepath.Join(dir, "current.jsonl"))
	slot := &sessionSlot{sess: m.sess}
	m.sessSlot = slot
	m.confirm = &confirmPolicy{always: map[string]bool{"bash": true}}
	m.usageAnchor = nil
	m.lastCompaction = nil
	m.contextTokens = 123
	m.busy = true
	m.compacting = true

	m.performSessionSwitch("target")

	if m.sess == nil || m.sess.Path() != targetPath {
		t.Fatalf("sess = %v, want %s", m.sess, targetPath)
	}
	if slot.sess != m.sess || !slot.switched {
		t.Fatalf("slot = %+v, want the switched session", slot)
	}
	if m.model != "target-model" {
		t.Fatalf("model = %q, want target-model", m.model)
	}
	if m.provider != "old-provider" {
		t.Fatalf("provider = %q, want old-provider", m.provider)
	}
	if m.system != "stored system prompt" {
		t.Fatalf("system = %q", m.system)
	}
	if len(m.history) != 3 || m.history[0].Role != "system" || m.history[1].Content != "hello" {
		t.Fatalf("history = %+v", m.history)
	}
	if m.contextTokens == 0 || m.contextSize == 0 {
		t.Fatalf("context accounting not recomputed: tokens=%d window=%d", m.contextTokens, m.contextSize)
	}
	if m.usageAnchor != nil || m.lastCompaction != nil || m.compactCooldown != 0 {
		t.Fatal("context accounting was not reset")
	}
	if m.busy || m.compacting {
		t.Fatal("transient turn state was not reset")
	}
	if m.confirm != nil && len(m.confirm.always) != 0 {
		t.Fatalf("tool approvals leaked across sessions: %v", m.confirm.always)
	}
	if got := m.messages[len(m.messages)-1].content; !strings.Contains(got, "Switched to session Target work (target)") {
		t.Fatalf("switch notice = %q", got)
	}
	if !strings.Contains(m.renderMessages(), "hello") {
		t.Fatal("resumed transcript was not rendered")
	}
}

func TestSwitchSessionRefusesWhileBusy(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	withSessionGlobals(t)

	m := switchTestModel()
	defer closeTestSession(m)
	m.sessionDirPath = t.TempDir()
	m.state = tuiProcessing

	m.switchSession("anything")
	if m.pendingSwitch != nil {
		t.Fatal("busy switch created a pending prompt")
	}
	if got := m.messages[len(m.messages)-1].content; !strings.Contains(got, "Interrupt the current turn") {
		t.Fatalf("busy notice = %q", got)
	}
}

func TestSwitchSessionPromptsForUnsavedChat(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	withSessionGlobals(t)

	dir := t.TempDir()
	createSessionFile(t, dir, "target", "", "sys", "m", types.ChatMessage{Role: "user", Content: "target"})

	m := switchTestModel()
	defer closeTestSession(m)
	m.sessionDirPath = dir
	m.sess = nil
	m.history = []types.ChatMessage{{Role: "system", Content: "sys"}, {Role: "user", Content: "unsaved"}}

	m.switchSession("target")
	if m.pendingSwitch == nil {
		t.Fatal("switchSession() did not prompt for an unsaved chat")
	}
	if m.sess != nil {
		t.Fatalf("sess switched before the prompt was answered: %v", m.sess)
	}
	if got := m.messages[len(m.messages)-1].content; !strings.Contains(got, "not saved") {
		t.Fatalf("prompt = %q", got)
	}

	_, _ = m.handlePendingSessionSwitchKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.pendingSwitch != nil {
		t.Fatal("discard left the prompt open")
	}
	if m.sess == nil || m.sess.ID() != "target" {
		t.Fatalf("discard did not switch: %v", m.sess)
	}
}

func TestSwitchSessionSavePromptPersistsChat(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	withSessionGlobals(t)

	dir := t.TempDir()
	createSessionFile(t, dir, "target", "", "sys", "m", types.ChatMessage{Role: "user", Content: "target"})

	m := switchTestModel()
	defer closeTestSession(m)
	m.sessionDirPath = dir
	m.sess = nil
	m.history = []types.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "save me"},
		{Role: "assistant", Content: "saved"},
	}

	m.switchSession("target")
	_, _ = m.handlePendingSessionSwitchKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})

	if m.pendingSwitch != nil || m.sess == nil || m.sess.ID() != "target" {
		t.Fatalf("save + switch failed: pending=%v sess=%v", m.pendingSwitch, m.sess)
	}
	list, err := session.List(dir, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("saved sessions = %d, want 2 (target + saved chat)", len(list))
	}
	var saved *session.Summary
	for i := range list {
		if list[i].ID != "target" {
			saved = &list[i]
		}
	}
	if saved == nil || saved.Messages != 2 {
		t.Fatalf("saved chat = %+v", saved)
	}
	loaded, err := session.Load(saved.Path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.System() != "sys" || len(loaded.History()) != 3 {
		t.Fatalf("saved history = %+v", loaded.History())
	}
}

func TestSwitchSessionCancelKeepsCurrent(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	withSessionGlobals(t)

	dir := t.TempDir()
	createSessionFile(t, dir, "target", "", "sys", "m", types.ChatMessage{Role: "user", Content: "target"})

	m := switchTestModel()
	defer closeTestSession(m)
	m.sessionDirPath = dir
	m.sess = nil
	m.history = []types.ChatMessage{{Role: "user", Content: "unsaved"}}
	m.switchSession("target")

	_, _ = m.handlePendingSessionSwitchKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.pendingSwitch != nil || m.sess != nil {
		t.Fatalf("cancel changed state: pending=%v sess=%v", m.pendingSwitch, m.sess)
	}
	if got := m.messages[len(m.messages)-1].content; !strings.Contains(got, "cancelled") {
		t.Fatalf("cancel notice = %q", got)
	}
}

func TestResolveSessionTargetByIndexIDAndPath(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	withSessionGlobals(t)

	dir := t.TempDir()
	now := time.Now()
	older := createSessionFile(t, dir, "older", "", "", "", types.ChatMessage{Role: "user", Content: "old"})
	newer := createSessionFile(t, dir, "newer", "", "", "", types.ChatMessage{Role: "user", Content: "new"})
	if err := os.Chtimes(older, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, now, now); err != nil {
		t.Fatal(err)
	}

	m := switchTestModel()
	defer closeTestSession(m)
	m.sessionDirPath = dir

	if got, err := m.resolveSessionTarget("1"); err != nil || got != newer {
		t.Fatalf("index 1 = %q, %v; want %q", got, err, newer)
	}
	if got, err := m.resolveSessionTarget("older"); err != nil || got != older {
		t.Fatalf("id = %q, %v; want %q", got, err, older)
	}
	if got, err := m.resolveSessionTarget(older); err != nil || got != older {
		t.Fatalf("path = %q, %v; want %q", got, err, older)
	}
	if _, err := m.resolveSessionTarget("99"); err == nil {
		t.Fatal("index 99 did not fail")
	}
	if _, err := m.resolveSessionTarget("missing"); err == nil {
		t.Fatal("missing id did not fail")
	}
}

func TestSwitchSessionExplicitModelWins(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	withSessionGlobals(t)
	options.Shared.Model = "explicit-model"

	dir := t.TempDir()
	createSessionFile(t, dir, "target", "", "sys", "target-model", types.ChatMessage{Role: "user", Content: "hello"})

	m := switchTestModel()
	defer closeTestSession(m)
	m.model = "explicit-model"
	m.sessionDirPath = dir
	m.performSessionSwitch("target")

	if m.model != "explicit-model" {
		t.Fatalf("model = %q, want explicit-model", m.model)
	}
}

func TestSwitchSessionAppliesNamedProvider(t *testing.T) {
	cfg := &types.Config{Providers: map[string]*types.NamedProvider{
		"alt": {Type: types.ProviderOpenAI, BaseURL: "https://alt.example/v1", Model: "alt-model"},
	}}
	withChatConfig(t, cfg, 0, 0, 0)
	withSessionGlobals(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "target.jsonl")
	s, err := session.New(path, session.Header{ID: "target", Provider: "alt", Model: "alt-model"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AppendMessage(types.ChatMessage{Role: "user", Content: "hello"}, "", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.Close("test"); err != nil {
		t.Fatal(err)
	}

	m := switchTestModel()
	defer closeTestSession(m)
	m.sessionDirPath = dir
	m.performSessionSwitch("target")

	if m.provider != "alt" || m.model != "alt-model" {
		t.Fatalf("selection = %q/%q, want alt/alt-model", m.provider, m.model)
	}
	if m.client == nil {
		t.Fatal("client was not rebuilt for the new provider")
	}
}

func TestSwitchSessionUnresolvedProviderKeepsCurrentProvider(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	withSessionGlobals(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "target.jsonl")
	s, err := session.New(path, session.Header{ID: "target", Provider: "missing", Model: "target-model"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AppendMessage(types.ChatMessage{Role: "user", Content: "hello"}, "", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.Close("test"); err != nil {
		t.Fatal(err)
	}

	m := switchTestModel()
	defer closeTestSession(m)
	m.sessionDirPath = dir
	m.performSessionSwitch("target")

	if m.provider != "old-provider" || m.model != "target-model" {
		t.Fatalf("selection = %q/%q, want old-provider/target-model (resume-compatible fallback)", m.provider, m.model)
	}
}

func TestApplySessionStateSamePathIsNoop(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	withSessionGlobals(t)

	dir := t.TempDir()
	path := createSessionFile(t, dir, "only", "", "sys", "m", types.ChatMessage{Role: "user", Content: "hello"})

	m := switchTestModel()
	defer closeTestSession(m)
	m.sessionDirPath = dir
	m.sess = openSession(t, path)
	before := len(m.messages)
	m.performSessionSwitch("only")
	if len(m.messages) != before+1 {
		t.Fatalf("same-path switch appended %d messages", len(m.messages)-before)
	}
	if got := m.messages[len(m.messages)-1].content; !strings.Contains(got, "Already in session") {
		t.Fatalf("same-path notice = %q", got)
	}
}

func TestHandleSubmitSessionCommandSwitches(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	withSessionGlobals(t)

	dir := t.TempDir()
	createSessionFile(t, dir, "current", "", "sys", "m", types.ChatMessage{Role: "user", Content: "old"})
	createSessionFile(t, dir, "target", "", "sys", "m", types.ChatMessage{Role: "user", Content: "new"})

	m := switchTestModel()
	defer closeTestSession(m)
	m.sessionDirPath = dir
	m.sess = openSession(t, filepath.Join(dir, "current.jsonl"))
	m.input.SetValue("/session target")
	_, _ = m.handleSubmitInput()

	if m.sess == nil || m.sess.ID() != "target" {
		t.Fatalf("/session did not switch: %v", m.sess)
	}
	if got := m.input.Value(); got != "" {
		t.Fatalf("/session left input behind: %q", got)
	}
}

func TestSaveCommandPersistsChat(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	withSessionGlobals(t)

	dir := t.TempDir()
	m := switchTestModel()
	defer closeTestSession(m)
	m.sessionDirPath = dir
	m.sess = nil
	m.input.SetValue("/save my title")
	_, _ = m.handleSubmitInput()

	if m.sess == nil {
		t.Fatal("/save did not create a session")
	}
	if got := m.input.Value(); got != "" {
		t.Fatalf("/save left input behind: %q", got)
	}
	loaded, err := session.Load(m.sess.Path())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name() != "my title" {
		t.Fatalf("name = %q, want my title", loaded.Name())
	}
	if loaded.System() != "sys" || len(loaded.History()) != 2 {
		t.Fatalf("saved history = %+v (system %q)", loaded.History(), loaded.System())
	}
	list, err := session.List(dir, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("listed sessions = %+v, %v", list, err)
	}

	m.input.SetValue("/save")
	_, _ = m.handleSubmitInput()
	if got := m.messages[len(m.messages)-1].content; !strings.Contains(got, "Already saving") {
		t.Fatalf("second /save notice = %q", got)
	}
}

func TestSaveCurrentChatRecordsExplicitAPIDefault(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	withSessionGlobals(t)

	dir := t.TempDir()
	m := switchTestModel()
	defer closeTestSession(m)
	m.sessionDirPath = dir
	m.sess = nil
	m.model = ""
	m.history = []types.ChatMessage{{Role: "system", Content: "sys"}, {Role: "user", Content: "hello"}}

	saved, err := m.saveCurrentChat()
	if err != nil {
		t.Fatal(err)
	}
	_ = saved.Close("test")

	loaded, err := session.Load(saved.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.HasModelSelection() || loaded.Header().Model != "" {
		t.Fatalf("API-default selection was not recorded: has=%v model=%q",
			loaded.HasModelSelection(), loaded.Header().Model)
	}
}

func TestShowSessionListReportsTotal(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	withSessionGlobals(t)

	dir := t.TempDir()
	now := time.Now()
	for i := 0; i < sessionListLimit+1; i++ {
		id := fmt.Sprintf("s%02d", i)
		path := createSessionFile(t, dir, id, "", "", "m", types.ChatMessage{Role: "user", Content: "message " + id})
		at := now.Add(-time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}

	m := switchTestModel()
	defer closeTestSession(m)
	m.sessionDirPath = dir
	m.showSessionList()
	out := m.messages[len(m.messages)-1].content
	if !strings.Contains(out, fmt.Sprintf("Showing the %d most recent of %d sessions", sessionListLimit, sessionListLimit+1)) {
		t.Fatalf("total footer missing:\n%s", out)
	}
	if !strings.Contains(out, "/session find <text>") {
		t.Fatalf("search hint missing:\n%s", out)
	}
}

func TestShowSessionSearchFindsOlderSession(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	withSessionGlobals(t)

	dir := t.TempDir()
	now := time.Now()
	for i := 0; i < sessionListLimit; i++ {
		id := fmt.Sprintf("recent%02d", i)
		path := createSessionFile(t, dir, id, "", "", "m", types.ChatMessage{Role: "user", Content: "recent"})
		at := now.Add(-time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	needle := createSessionFile(t, dir, "needle-id", "needle topic", "", "m",
		types.ChatMessage{Role: "user", Content: "unique haystack text"})
	oldest := now.Add(-24 * time.Hour)
	if err := os.Chtimes(needle, oldest, oldest); err != nil {
		t.Fatal(err)
	}

	m := switchTestModel()
	defer closeTestSession(m)
	m.sessionDirPath = dir
	m.showSessionSearch("needle")
	out := m.messages[len(m.messages)-1].content
	if !strings.Contains(out, "needle-id") || !strings.Contains(out, "needle topic") {
		t.Fatalf("search did not find the old session:\n%s", out)
	}
	if !strings.Contains(out, "/session <id|path>") {
		t.Fatalf("switch hint missing:\n%s", out)
	}

	m.showSessionSearch("unique haystack")
	out = m.messages[len(m.messages)-1].content
	if !strings.Contains(out, "needle-id") {
		t.Fatalf("search by first message failed:\n%s", out)
	}
}

func TestFilterSessionSummaries(t *testing.T) {
	list := []session.Summary{
		{ID: "a", Name: "Auth review", FirstUser: "token refresh", Model: "claude", Cwd: "/work/api"},
		{ID: "b", Name: "", FirstUser: "cyberpunk cat", Model: "gpt-image", Cwd: "/work/media"},
	}
	cases := map[string]string{
		"auth":        "a",
		"token":       "a",
		"/work/media": "b",
		"gpt-image":   "b",
		"AUTH":        "a",
	}
	for query, want := range cases {
		got := filterSessionSummaries(list, query)
		if len(got) != 1 || got[0].ID != want {
			t.Errorf("filterSessionSummaries(%q) = %+v, want id %s", query, got, want)
		}
	}
	if got := filterSessionSummaries(list, "  "); len(got) != len(list) {
		t.Errorf("empty query returned %d, want %d", len(got), len(list))
	}
	if got := filterSessionSummaries(list, "no-such-thing"); len(got) != 0 {
		t.Errorf("non-matching query returned %+v", got)
	}
}

func TestResolveSessionTargetReportsHiddenSessions(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	withSessionGlobals(t)

	dir := t.TempDir()
	now := time.Now()
	for i := 0; i < sessionListLimit+1; i++ {
		id := fmt.Sprintf("s%02d", i)
		path := createSessionFile(t, dir, id, "", "", "m", types.ChatMessage{Role: "user", Content: "hi"})
		at := now.Add(-time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}

	m := switchTestModel()
	defer closeTestSession(m)
	m.sessionDirPath = dir
	_, err := m.resolveSessionTarget("21")
	if err == nil || !strings.Contains(err.Error(), "outside the newest 20") || !strings.Contains(err.Error(), "find") {
		t.Fatalf("resolveSessionTarget(21) = %v, want hidden-session hint", err)
	}
}

func TestFormatSessionAge(t *testing.T) {
	now := time.Now()
	cases := map[string]time.Time{
		"now":  now.Add(-10 * time.Second),
		"5m":   now.Add(-5 * time.Minute),
		"3h":   now.Add(-3 * time.Hour),
		"2d":   now.Add(-50 * time.Hour),
		"date": now.Add(-30 * 24 * time.Hour),
	}
	for want, at := range cases {
		got := formatSessionAge(at)
		if want == "date" {
			if got != at.Local().Format("2006-01-02") {
				t.Errorf("formatSessionAge(old) = %q", got)
			}
			continue
		}
		if !strings.HasPrefix(got, want) {
			t.Errorf("formatSessionAge(%v) = %q, want prefix %q", at, got, want)
		}
	}
}
