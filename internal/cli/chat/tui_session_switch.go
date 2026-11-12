package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/session"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// sessionListLimit is how many recent sessions /session shows. Older entries
// remain reachable by id or path; a full picker with scrolling can raise this
// later without changing the resolution rules.
const sessionListLimit = 20

// sessionSlot keeps the current session reachable from the TUI's exit handler.
// The handler must close whatever session is active at exit, not the one that
// happened to be open at startup.
type sessionSlot struct {
	sess     *session.Session
	switched bool
}

// pendingSessionSwitch waits for the user to decide what to do with an unsaved
// chat before switching away from it.
type pendingSessionSwitch struct {
	arg string
}

// showSessionList prints the recent saved sessions and marks the current one.
func (m *chatModel) showSessionList() {
	dir := m.sessionDir()
	list, err := session.List(dir, sessionListLimit)
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Cannot list sessions: %v", err))
		return
	}
	if len(list) == 0 {
		m.addSystemMessage(fmt.Sprintf("No saved sessions in %s.\nStart one with --save, then /session can switch between them.", dir))
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Sessions in %s\n", dir)
	current := m.currentSessionPath()
	for i, s := range list {
		marker := sessionMarker(s, current)
		name := truncateTitle(s.DisplayName(), 28)
		fmt.Fprintf(&b, "%s%2d. %s  %s\n", marker, i+1, padRight(name, 28), sessionMeta(s))
		if s.FirstUser != "" {
			fmt.Fprintf(&b, "     %s\n", truncateTitle(s.FirstUser, 64))
		}
	}
	b.WriteString("\nUse /session <number|id|path> to switch.")
	if total, err := session.Count(dir); err == nil && total > len(list) {
		fmt.Fprintf(&b, "\nShowing the %d most recent of %d sessions; use /session find <text> to search older ones.", len(list), total)
	}
	m.addSystemMessage(strings.TrimRight(b.String(), "\n"))
}

// showSessionSearch scans every saved session (not just the newest page) for a
// case-insensitive substring in its name, id, first message, model, provider,
// cwd or path. Matches are identified by id because the numbering of the
// default list does not apply to a filtered view.
func (m *chatModel) showSessionSearch(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		m.showSessionList()
		return
	}
	dir := m.sessionDir()
	all, err := session.List(dir, 0)
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Cannot search sessions: %v", err))
		return
	}
	matches := filterSessionSummaries(all, query)
	if len(matches) == 0 {
		m.addSystemMessage(fmt.Sprintf("No sessions matching %q in %s.", query, dir))
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Sessions matching %q (%d)\n", query, len(matches))
	current := m.currentSessionPath()
	const maxMatches = 20
	for i, s := range matches {
		if i >= maxMatches {
			fmt.Fprintf(&b, "  ... %d more match; narrow the query.\n", len(matches)-maxMatches)
			break
		}
		label := s.ID
		if s.Name != "" && s.Name != s.ID {
			label = truncateTitle(s.Name, 32) + " (" + s.ID + ")"
		}
		fmt.Fprintf(&b, "%s  %s  %s\n", sessionMarker(s, current), label, sessionMeta(s))
		if s.FirstUser != "" {
			fmt.Fprintf(&b, "     %s\n", truncateTitle(s.FirstUser, 64))
		}
	}
	b.WriteString("\nUse /session <id|path> to switch.")
	m.addSystemMessage(strings.TrimRight(b.String(), "\n"))
}

// filterSessionSummaries keeps the summaries whose searchable fields contain
// query. It is pure so the matching rules are testable without a TUI.
func filterSessionSummaries(list []session.Summary, query string) []session.Summary {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return list
	}
	out := make([]session.Summary, 0, len(list))
	for _, s := range list {
		haystack := strings.ToLower(strings.Join([]string{
			s.Name, s.ID, s.Path, s.Cwd, s.Provider, s.Model, s.FirstUser,
		}, "\n"))
		if strings.Contains(haystack, q) {
			out = append(out, s)
		}
	}
	return out
}

func (m *chatModel) currentSessionPath() string {
	if m.sess == nil {
		return ""
	}
	return m.sess.Path()
}

func sessionMarker(s session.Summary, currentPath string) string {
	if currentPath != "" && filepath.Clean(s.Path) == filepath.Clean(currentPath) {
		return "* "
	}
	return "  "
}

func sessionMeta(s session.Summary) string {
	model := s.Model
	if model == "" {
		model = "<API default>"
	}
	return fmt.Sprintf("%s  ·  %d msgs  ·  %s",
		padRight(formatSessionAge(s.UpdatedAt), 8), s.Messages, truncateTitle(model, 32))
}

// switchSession handles /session with an argument. Switching away from an
// unsaved conversation asks first; an empty chat has nothing to lose.
func (m *chatModel) switchSession(arg string) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		m.showSessionList()
		return
	}
	if m.state != tuiIdle || m.busy || m.compacting {
		m.addSystemMessage("Interrupt the current turn before switching sessions.")
		return
	}
	if m.sess == nil && sessionHasConversation(m.history) {
		m.pendingSwitch = &pendingSessionSwitch{arg: arg}
		m.addSystemMessage("Current chat is not saved.\n  [s] Save and switch\n  [d] Discard and switch\n  [c] Cancel")
		return
	}
	m.performSessionSwitch(arg)
}

// handlePendingSessionSwitchKey owns the keyboard while the save/discard
// prompt is open. It runs before the confirmation prompt and the input box.
func (m *chatModel) handlePendingSessionSwitchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	pending := m.pendingSwitch
	if pending == nil {
		return m, nil
	}
	switch msg.String() {
	case "s", "S", "y", "Y":
		saved, err := m.saveCurrentChat()
		if err != nil {
			m.pendingSwitch = nil
			m.addSystemMessage(fmt.Sprintf("Save failed: %v", err))
			return m, nil
		}
		m.pendingSwitch = nil
		m.addSystemMessage(fmt.Sprintf("Saved current chat as %s — %s", saved.ID(), saved.Path()))
		m.performSessionSwitch(pending.arg)
	case "d", "D", "n", "N":
		m.pendingSwitch = nil
		m.performSessionSwitch(pending.arg)
	case "c", "C", "esc", "ctrl+c":
		m.pendingSwitch = nil
		m.addSystemMessage("Session switch cancelled.")
	}
	return m, nil
}

// performSessionSwitch opens the target, applies its stored state and closes
// the previous session. The target is opened before the old session is closed,
// so a bad id leaves the current conversation untouched.
func (m *chatModel) performSessionSwitch(arg string) {
	path, err := m.resolveSessionTarget(arg)
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Cannot switch session: %v", err))
		return
	}
	if m.sess != nil && filepath.Clean(m.sess.Path()) == filepath.Clean(path) {
		m.addSystemMessage(fmt.Sprintf("Already in session %s.", m.sess.ID()))
		return
	}
	target, err := session.Open(path)
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Cannot open session %s: %v", path, err))
		return
	}

	old := m.sess
	m.applySessionState(target)
	if old != nil {
		if err := old.Close("switch"); err != nil {
			m.addSystemMessage(fmt.Sprintf("Warning: closing the previous session failed: %v", err))
		}
	}
	m.addSystemMessage(switchedNotice(target))
}

// saveChat handles /save. With no session file it promotes the in-memory
// conversation to a new session; on an already-saved session it only reports
// the path, or renames it when a name is given.
func (m *chatModel) saveChat(name string) {
	name = strings.TrimSpace(name)
	if m.state != tuiIdle || m.busy || m.compacting {
		m.addSystemMessage("Finish or interrupt the current turn before saving.")
		return
	}
	if m.sess != nil {
		if name != "" {
			m.setSessionName(name)
			return
		}
		m.addSystemMessage(fmt.Sprintf("Already saving to %s — %s\nResume: aigc-cli chat --session %s",
			m.sess.ID(), m.sess.Path(), m.sess.ID()))
		return
	}
	if !sessionHasConversation(m.history) {
		m.addSystemMessage("Nothing to save yet; send a message first.")
		return
	}

	saved, err := m.saveCurrentChat()
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Save failed: %v", err))
		return
	}
	if name != "" {
		if err := saved.AppendName(name); err != nil {
			m.addSystemMessage(fmt.Sprintf("Saved, but naming failed: %v", err))
		}
	}
	label := saved.ID()
	if n := saved.Name(); n != "" {
		label = n + " (" + saved.ID() + ")"
	}
	m.addSystemMessage(fmt.Sprintf("Saved current chat as %s — %s\nResume: aigc-cli chat --session %s",
		label, saved.Path(), saved.ID()))
}

// saveCurrentChat promotes the in-memory conversation to a saved session. The
// new file stays open so a failed switch still leaves the chat persisted.
func (m *chatModel) saveCurrentChat() (*session.Session, error) {
	dir := m.sessionDir()
	header := currentSessionHeader()
	header.ID = session.NewID(time.Now())
	header.Provider = m.provider
	header.Model = m.model
	header.ContextSize = m.compactSettings.Window
	if header.ContextSize <= 0 {
		header.ContextSize = m.contextSize
	}
	header.ContextSizeSource = "auto"
	if chatContextSize > 0 {
		header.ContextSizeSource = "explicit"
	}

	path := filepath.Join(dir, header.ID+".jsonl")
	s, err := session.New(path, header)
	if err != nil {
		return nil, err
	}
	cleanup := func() {
		_ = s.Close("error")
		_ = os.Remove(path)
	}

	m.mu.Lock()
	history := append([]types.ChatMessage(nil), m.history...)
	m.mu.Unlock()

	sys := m.system
	if len(history) > 0 && history[0].Role == "system" {
		if sys == "" {
			sys = history[0].Content
		}
		history = history[1:]
	}
	if sys != "" {
		if err := s.AppendSystem(sys); err != nil {
			cleanup()
			return nil, err
		}
	}
	for _, msg := range history {
		if err := s.AppendMessage(msg, "", m.toolOutputMax); err != nil {
			cleanup()
			return nil, err
		}
	}
	if name := autoSessionName(history); name != "" {
		if err := s.AppendName(name); err != nil {
			cleanup()
			return nil, err
		}
	}
	// An empty model means "let the API/provider choose". A header alone cannot
	// express that distinction on resume (empty falls back to the provider's
	// configured model), so record it as an explicit selection.
	if m.model == "" {
		if err := s.AppendModel(m.provider, "", header.ContextSize, header.ContextSizeSource); err != nil {
			cleanup()
			return nil, err
		}
	}

	m.sess = s
	if m.sessSlot != nil {
		m.sessSlot.sess = s
		m.sessSlot.switched = true
	}
	return s, nil
}

// applySessionState replaces every per-session field on the model. Missing one
// here leaks state between conversations, so the list is intentionally
// exhaustive: history, prompt, provider/model, context accounting, transient
// turn state and the session-scoped tool approvals.
func (m *chatModel) applySessionState(target *session.Session) {
	cfg := options.ChatDefaults()
	h := target.Header()

	providerName, model := m.provider, m.model
	explicitProvider := options.Shared.ProviderSet || (cfg != nil && cfg.Provider != "")
	explicitModel := options.Shared.Model != "" || (cfg != nil && cfg.Model != "")

	if !explicitProvider && h.Provider != "" {
		if p := options.Shared.ResolveNamedProvider(h.Provider); p != nil {
			if name := providerDisplayName(p); name != "" {
				providerName = name
			}
		}
	}
	if !explicitModel && (h.Model != "" || target.HasModelSelection()) {
		model = h.Model
	}

	if providerName != m.provider {
		p := options.Shared.ResolveNamedProvider(providerName)
		switch {
		case p == nil:
			providerName, model = m.provider, m.model
		case p.Type == types.ProviderLocal:
			providerName, model = m.provider, m.model
		case options.RequireBaseURL(options.ProviderNameChat, p) != nil:
			providerName, model = m.provider, m.model
		default:
			m.client = client.NewFromProvider(p)
		}
	}

	sys := target.System()
	if sys == "" {
		sys = buildTUISystemPrompt()
	}
	history := target.History()
	if sys != "" && (len(history) == 0 || history[0].Role != "system") {
		history = append([]types.ChatMessage{{Role: "system", Content: sys}}, history...)
	}

	settings := resolveCompactionSettingsWithDetect(m.cmd, cfg, target, providerName, model, false, false)

	m.sess = target
	m.provider = providerName
	m.model = model
	m.system = sys
	m.history = history
	m.compactSettings = settings
	m.contextSize = settings.Window
	m.contextTokens = agent.EstimateContext(history, nil, m.toolsTokens)
	m.usageAnchor = nil
	m.lastCompaction = nil
	m.compactCooldown = 0
	m.messages = []message{{role: "system", content: sessionDivider(target)}}
	m.messages = append(m.messages, resumedChatMessages(history)...)
	m.streamBuf.Reset()
	m.err = nil
	m.pending = nil
	m.pendingSwitch = nil
	m.suggesting = false
	m.draft = ""
	m.busy = false
	m.compacting = false
	m.turnID++

	if m.confirm != nil {
		m.confirm.resetSession()
	}
	m.input.Reset()
	m.input.Focus()

	if m.sessSlot != nil {
		m.sessSlot.sess = target
		m.sessSlot.switched = true
	}
	if chatSystem != "" && target.System() != "" && chatSystem != target.System() {
		m.messages = append(m.messages, message{
			role:    "system",
			content: "Using the system prompt stored in this session; --system does not override an existing session.",
		})
	}
	m.refreshViewport()
	m.viewport.GotoBottom()
}

// resolveSessionTarget accepts a 1-based list index, an id under the session
// directory, or a path. Numeric ids are ambiguous with indexes; indexes win,
// which matches what /session just printed.
func (m *chatModel) resolveSessionTarget(arg string) (string, error) {
	dir := m.sessionDir()
	if n, err := strconv.Atoi(arg); err == nil {
		list, err := session.List(dir, sessionListLimit)
		if err != nil {
			return "", err
		}
		if n < 1 || n > len(list) {
			if n > len(list) {
				if total, cerr := session.Count(dir); cerr == nil && total > len(list) {
					return "", fmt.Errorf("session #%d is outside the newest %d; use /session <id|path> or /session find <text>", n, len(list))
				}
			}
			return "", fmt.Errorf("session #%d not found (%d listed)", n, len(list))
		}
		return list[n-1].Path, nil
	}
	target, err := session.ResolveExisting(expandHome(arg), dir)
	if err != nil {
		return "", err
	}
	return target.Path, nil
}

func (m *chatModel) sessionDir() string {
	if m.sessionDirPath != "" {
		return m.sessionDirPath
	}
	return sessionDir(options.ChatDefaults())
}

func (m *chatModel) addSystemMessage(content string) {
	m.messages = append(m.messages, message{role: "system", content: content})
	m.refreshViewport()
}

func sessionHasConversation(history []types.ChatMessage) bool {
	for _, msg := range history {
		if msg.Role != "system" {
			return true
		}
	}
	return false
}

func sessionDivider(sess *session.Session) string {
	label := sess.ID()
	if name := sess.Name(); name != "" {
		label = name + " (" + sess.ID() + ")"
	}
	return fmt.Sprintf("── Session %s — %s ──", label, sess.Path())
}

func switchedNotice(sess *session.Session) string {
	label := sess.ID()
	if name := sess.Name(); name != "" {
		label = name + " (" + sess.ID() + ")"
	}
	return fmt.Sprintf("Switched to session %s — %s", label, sess.Path())
}

func padRight(s string, width int) string {
	if w := ansi.StringWidth(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

func formatSessionAge(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Local().Format("2006-01-02")
	}
}
