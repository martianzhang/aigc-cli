package chat

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/session"
	"github.com/martianzhang/aigc-cli/internal/types"
)

const maxResumeDisplayMessages = 100

// sessionTitle is the banner label: the session name when set, the session id
// when a session is active but unnamed, and the original product label when
// chat is ephemeral.
func (m *chatModel) sessionTitle() string {
	if m.sess == nil {
		return "Chat with AI"
	}
	if name := m.sess.Name(); name != "" {
		return name
	}
	return "Session " + m.sess.ID()
}

// showSessionName prints the current session identity and name.
func (m *chatModel) showSessionName() {
	if m.sess == nil {
		m.messages = append(m.messages, message{
			role:    "system",
			content: "No active session. Start with --save or --session <id|file>.",
		})
		m.refreshViewport()
		return
	}
	name := m.sess.Name()
	if name == "" {
		name = "(unnamed)"
	}
	m.messages = append(m.messages, message{
		role: "system",
		content: fmt.Sprintf("Session: %s\nName:    %s\nPath:    %s",
			m.sess.ID(), name, m.sess.Path()),
	})
	m.refreshViewport()
}

// setSessionName renames the active session and persists the new name.
func (m *chatModel) setSessionName(name string) {
	if m.sess == nil {
		m.messages = append(m.messages, message{
			role:    "system",
			content: "No active session. Start with --save or --session <id|file>.",
		})
		m.refreshViewport()
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		m.showSessionName()
		return
	}
	if err := m.sess.AppendName(name); err != nil {
		m.messages = append(m.messages, message{
			role:    "system",
			content: fmt.Sprintf("Rename failed: %v", err),
		})
	} else {
		m.messages = append(m.messages, message{
			role:    "system",
			content: fmt.Sprintf("Session name set to %q.", m.sess.Name()),
		})
	}
	m.refreshViewport()
}

// sessionDisplay is the compact session indicator shown in the TUI status bar.
func (m *chatModel) sessionDisplay() string {
	if m.sess == nil {
		return ""
	}
	return "rec: " + filepath.Base(m.sess.Path())
}

// resumedChatMessages converts the projected history into TUI messages. It
// skips the system prompt (already represented by the status bar) and keeps the
// most recent entries so a huge session does not stall the first render.
func resumedChatMessages(history []types.ChatMessage) []message {
	start := 0
	if len(history) > 0 && history[0].Role == "system" {
		start = 1
	}

	out := make([]message, 0, len(history))
	for _, msg := range history[start:] {
		switch msg.Role {
		case "user":
			out = append(out, message{role: "user", content: msg.Content})
		case "assistant":
			if strings.TrimSpace(msg.Content) == "" && len(msg.ToolCalls) > 0 {
				names := make([]string, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					names = append(names, tc.Function.Name)
				}
				out = append(out, message{role: "system", content: "🔧 " + strings.Join(names, ", ")})
				continue
			}
			out = append(out, message{role: "assistant", content: msg.Content})
		case "tool":
			out = append(out, message{
				role:        "tool",
				tool:        "tool",
				toolSummary: firstLine(msg.Content),
				content:     msg.Content,
			})
		case "system":
			out = append(out, message{role: "system", content: msg.Content})
		}
	}

	if len(out) > maxResumeDisplayMessages {
		out = out[len(out)-maxResumeDisplayMessages:]
		out = append([]message{{
			role:    "system",
			content: fmt.Sprintf("Showing the last %d messages; the full history remains in the session file.", maxResumeDisplayMessages),
		}}, out...)
	}
	return out
}

func resumedNotice(sess *session.Session, count int) message {
	return message{
		role:    "system",
		content: fmt.Sprintf("Resumed session %s (%d messages) — %s", sess.ID(), count, sess.Path()),
	}
}

// buildTUISystemPrompt builds the interactive system prompt: current date,
// optional --system text, and AGENTS.md context.
func buildTUISystemPrompt() string {
	sysContent := fmt.Sprintf("今天是 %s。你只需要在用户明确询问日期时才回答日期，其他时候不要主动提及。", time.Now().Format("2006年1月2日"))
	if chatSystem != "" {
		sysContent += "\n" + chatSystem
	}
	if ctx := loadAgentsContext(); ctx != "" {
		sysContent += "\n\n" + ctx
	}
	return sysContent
}
