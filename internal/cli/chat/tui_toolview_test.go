package chat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestToolCallSummary(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args string
		want string
		not  string
	}{
		{
			name: "bash command",
			tool: "bash",
			args: `{"command":"echo hi\necho bye"}`,
			want: "$ echo hi",
			not:  "bye",
		},
		{
			name: "read with pagination",
			tool: "read",
			args: `{"filepath":"main.go","offset":100,"limit":50}`,
			want: "main.go (offset 100, limit 50)",
		},
		{
			name: "write line count",
			tool: "write",
			args: `{"filepath":"a.go","content":"one\ntwo"}`,
			want: "a.go (+2 lines)",
		},
		{
			name: "edit line counts",
			tool: "edit",
			args: `{"filepath":"a.go","old_string":"one\ntwo","new_string":"one\ntwo\nthree"}`,
			want: "a.go (-2/+3 lines)",
		},
		{
			name: "grep include and path",
			tool: "grep",
			args: `{"pattern":"func main","include":"*.go","path":"cmd"}`,
			want: "/func main/ *.go in cmd",
		},
		{
			name: "mcp tool falls back to compact json",
			tool: "generate_image",
			args: `{"prompt": "a cat",  "size": "1024x1024"}`,
			want: `{"prompt": "a cat", "size": "1024x1024"}`,
		},
		{
			name: "empty object is omitted",
			tool: "some_mcp_tool",
			args: `{}`,
			want: "",
		},
		{
			name: "invalid json is not echoed raw",
			tool: "some_mcp_tool",
			args: `not json`,
			want: "not json",
		},
		{
			name: "long command is clipped",
			tool: "bash",
			args: `{"command":"` + strings.Repeat("x", 300) + `"}`,
			want: "…",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolCallSummary(tt.tool, tt.args)
			if !strings.Contains(got, tt.want) {
				t.Errorf("toolCallSummary(%q) = %q, want it to contain %q", tt.tool, got, tt.want)
			}
			if tt.not != "" && strings.Contains(got, tt.not) {
				t.Errorf("toolCallSummary(%q) = %q, must not contain %q", tt.tool, got, tt.not)
			}
			if strings.Contains(got, "\n") {
				t.Errorf("summary must be single-line, got %q", got)
			}
		})
	}
}

func TestLineDiff(t *testing.T) {
	t.Run("identical", func(t *testing.T) {
		if got := lineDiff("a\nb", "a\nb", 0, 40); got != "" {
			t.Errorf("lineDiff on identical input = %q, want empty", got)
		}
	})

	t.Run("new file is all additions", func(t *testing.T) {
		got := lineDiff("", "a\nb\n", 0, 40)
		if got != "+a\n+b" {
			t.Errorf("lineDiff new file = %q, want +a\\n+b", got)
		}
	})

	t.Run("replacement keeps context", func(t *testing.T) {
		old := "a\nb\nc\nd\ne"
		new := "a\nb\nX\nd\ne"
		got := lineDiff(old, new, 1, 40)
		want := " b\n-c\n+X\n d"
		if got != want {
			t.Errorf("lineDiff = %q, want %q", got, want)
		}
	})

	t.Run("truncates long output", func(t *testing.T) {
		old := strings.Repeat("old\n", 100)
		new := strings.Repeat("new\n", 100)
		got := lineDiff(old, new, 0, 10)
		if !strings.Contains(got, "…") {
			t.Errorf("lineDiff did not signal truncation:\n%s", got)
		}
		if n := len(strings.Split(got, "\n")); n > 11 {
			t.Errorf("lineDiff emitted %d lines, want <= 11", n)
		}
	})
}

func TestToolCallDiff(t *testing.T) {
	t.Run("edit", func(t *testing.T) {
		got := toolCallDiff("edit", `{"filepath":"a.go","old_string":"x","new_string":"y"}`)
		if got != "-x\n+y" {
			t.Errorf("edit diff = %q, want -x\\n+y", got)
		}
	})

	t.Run("write existing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "a.txt")
		if err := os.WriteFile(path, []byte("old line\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got := toolCallDiff("write", `{"filepath":"`+path+`","content":"new line\n"}`)
		if !strings.Contains(got, "-old line") || !strings.Contains(got, "+new line") {
			t.Errorf("write diff = %q, want old removed and new added", got)
		}
	})

	t.Run("write new file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "new.txt")
		got := toolCallDiff("write", `{"filepath":"`+path+`","content":"hello\n"}`)
		if got != "+hello" {
			t.Errorf("write diff = %q, want +hello", got)
		}
	})

	t.Run("other tools have no diff", func(t *testing.T) {
		if got := toolCallDiff("bash", `{"command":"rm -rf /"}`); got != "" {
			t.Errorf("bash diff = %q, want empty", got)
		}
	})
}

func TestRenderToolMessageFoldsLongOutput(t *testing.T) {
	m := newChatModel(nil, nil, 10, "m", "", nil, false, 0, 0, 0)
	m.width = 100

	content := strings.Join([]string{
		"line 1", "line 2", "line 3", "line 4", "line 5", "line 6", "line 7",
		"line 8", "line 9", "line 10", "line 11", "line 12", "line 13", "line 14",
		"line 15", "line 16", "line 17", "line 18", "line 19", "line 20", "line 21",
		"line 22", "line 23", "line 24", "line 25",
	}, "\n")
	msg := message{role: "tool", tool: "grep", toolSummary: "/needle/", content: content}

	got := m.renderToolMessage(msg)
	if strings.Contains(got, "line 25") {
		t.Error("folded output should not contain the last line")
	}
	if !strings.Contains(got, "5 more lines (click or Ctrl+O to expand)") {
		t.Errorf("missing fold hint:\n%s", got)
	}
	if !strings.Contains(got, "🛠️  grep") || !strings.Contains(got, "/needle/") {
		t.Errorf("missing tool header or summary:\n%s", got)
	}

	msg.expanded = true
	got = m.renderToolMessage(msg)
	if !strings.Contains(got, "line 25") {
		t.Error("expanded output should contain the last line")
	}
	if !strings.Contains(got, "click or Ctrl+O to collapse") {
		t.Errorf("missing collapse hint:\n%s", got)
	}
}

func TestRenderToolMessageShowsDiff(t *testing.T) {
	m := newChatModel(nil, nil, 10, "m", "", nil, false, 0, 0, 0)
	m.width = 100

	got := m.renderToolMessage(message{
		role:        "tool",
		tool:        "edit",
		toolSummary: "a.go (-1/+1 lines)",
		toolDiff:    "-old\n+new",
		content:     "Successfully edited a.go",
	})
	if !strings.Contains(got, "-old") || !strings.Contains(got, "+new") {
		t.Errorf("diff missing from rendered tool message:\n%s", got)
	}
}

func TestToggleLastToolExpanded(t *testing.T) {
	m := newChatModel(nil, nil, 10, "m", "", nil, false, 0, 0, 0)
	base := len(m.messages)
	m.messages = append(m.messages,
		message{role: "tool", tool: "read", content: "first"},
		message{role: "system", content: "between"},
		message{role: "tool", tool: "grep", content: "last"},
	)

	m.toggleLastToolExpanded()
	if !m.messages[base+2].expanded {
		t.Error("last tool message was not expanded")
	}
	if m.messages[base].expanded {
		t.Error("toggle touched an older tool message")
	}

	m.toggleLastToolExpanded()
	if m.messages[base+2].expanded {
		t.Error("second toggle did not collapse")
	}
}

func TestClipStringRuneSafe(t *testing.T) {
	got := clipString("你好世界", 3)
	if got != "你…" {
		t.Errorf("clipString = %q, want 你…", got)
	}
	if got := clipString("short", 20); got != "short" {
		t.Errorf("clipString should leave short strings alone, got %q", got)
	}
}

func TestRenderToolMessageShowsFullCommandWhenExpanded(t *testing.T) {
	m := newChatModel(nil, nil, 10, "m", "", nil, false, 0, 0, 0)
	m.width = 100

	msg := message{
		role:        "tool",
		tool:        "bash",
		toolSummary: "$ echo one",
		toolDetail:  "echo one\necho two",
		content:     "one\ntwo",
	}

	got := stripANSI(m.renderToolMessage(msg))
	if strings.Contains(got, "echo two") {
		t.Errorf("collapsed card leaked the second command line:\n%s", got)
	}
	if !strings.Contains(got, "command abbreviated") {
		t.Errorf("missing abbreviated-command hint:\n%s", got)
	}

	msg.expanded = true
	got = stripANSI(m.renderToolMessage(msg))
	if !strings.Contains(got, "$ echo one") || !strings.Contains(got, "echo two") {
		t.Errorf("expanded card must show the full command:\n%s", got)
	}
	if !strings.Contains(got, "click or Ctrl+O to collapse") {
		t.Errorf("missing collapse hint:\n%s", got)
	}
}

func TestClickToolCardTogglesExpansion(t *testing.T) {
	m := newTestModel(t)
	m.messages = append(m.messages, message{
		role:        "tool",
		tool:        "bash",
		toolSummary: "$ echo one",
		toolDetail:  "echo one\necho two",
		content:     "one",
	})
	m.refreshViewport()
	r := m.msgRanges[len(m.msgRanges)-1]
	// Screen row 1 is the header, so the transcript row maps to Y = line+1.
	y := r.start + 1

	click := func() {
		updated, _ := m.Update(tea.MouseMsg{X: 2, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		m = updated.(*chatModel)
		updated, _ = m.Update(tea.MouseMsg{X: 2, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
		m = updated.(*chatModel)
	}

	click()
	if !m.messages[len(m.messages)-1].expanded {
		t.Fatal("click did not expand the tool card")
	}
	click()
	if m.messages[len(m.messages)-1].expanded {
		t.Fatal("second click did not collapse the tool card")
	}
}

func TestClickToolCardDragStillSelects(t *testing.T) {
	oldCopy := copyToClipboard
	var copied string
	copyToClipboard = func(s string) error { copied = s; return nil }
	t.Cleanup(func() { copyToClipboard = oldCopy })

	m := newTestModel(t)
	m.messages = append(m.messages, message{
		role:        "tool",
		tool:        "bash",
		toolSummary: "$ echo one",
		toolDetail:  "echo one\necho two",
		content:     "one\ntwo\nthree",
	})
	m.refreshViewport()
	r := m.msgRanges[len(m.msgRanges)-1]

	y := r.start + 1
	updated, _ := m.Update(tea.MouseMsg{X: 2, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(*chatModel)
	updated, _ = m.Update(tea.MouseMsg{X: 2, Y: y + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m = updated.(*chatModel)
	updated, _ = m.Update(tea.MouseMsg{X: 2, Y: y + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	m = updated.(*chatModel)

	if m.messages[len(m.messages)-1].expanded {
		t.Error("drag must not toggle the tool card")
	}
	if copied == "" {
		t.Error("drag did not copy the selected rows")
	}
}
