package chat

import (
	"errors"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func contextTestModel() *chatModel {
	m := newChatModel(nil, nil, 10, "test-model", "", nil, false, 0, 0, 128000)
	m.compactSettings = agent.Settings{
		Enabled: true, Window: 128000, ReserveTokens: 16384, KeepRecentTokens: 20000,
	}
	m.toolsTokens = 4000
	m.contextSize = 128000
	m.contextTokens = 51200
	m.width = 120
	return m
}

func TestContextPercent(t *testing.T) {
	m := contextTestModel()
	if got := m.contextPercent(); got != 40 {
		t.Fatalf("contextPercent() = %d, want 40", got)
	}
	m.contextSize = 0
	if got := m.contextPercent(); got != -1 {
		t.Fatalf("contextPercent() with unknown window = %d, want -1", got)
	}
}

func TestShowContext(t *testing.T) {
	m := contextTestModel()
	m.history = []types.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	m.lastCompaction = &agent.CompactResult{
		Reason: "threshold", TokensBefore: 98000, Tokens: 41000, TokenSaved: 57000,
		Summary: "summary", SummaryModel: "m", KeepLast: 12,
		Usage: &types.ChatUsage{PromptTokens: 96000, CompletionTokens: 1800, TotalTokens: 97800},
	}
	m.showContext()
	if len(m.messages) == 0 {
		t.Fatal("showContext() added no message")
	}
	text := m.messages[len(m.messages)-1].content
	for _, want := range []string{"Context usage", "51200 / 128000", "compact at", "last compaction", "usage"} {
		if !strings.Contains(text, want) {
			t.Fatalf("context output missing %q:\n%s", want, text)
		}
	}
}

func TestStatusBarShowsContextPercent(t *testing.T) {
	m := contextTestModel()
	got := m.renderStatus()
	if !strings.Contains(got, "ctx 40%") {
		t.Fatalf("renderStatus() = %q, want ctx 40%%", got)
	}
}

func TestCompactDoneKeepsTranscript(t *testing.T) {
	m := contextTestModel()
	m.messages = []message{
		{role: "system", content: "welcome"},
		{role: "user", content: "old question"},
		{role: "assistant", content: "old answer"},
	}
	newHistory := []types.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "system", Content: "summary"},
		{Role: "user", Content: "recent"},
	}
	next, _ := m.Update(compactDone{
		turn:           m.turnID,
		count:          5,
		kept:           1,
		summary:        "the summary",
		newHistory:     newHistory,
		lastCompaction: &agent.CompactResult{Reason: "manual", Tokens: 100, TokensBefore: 900, TokenSaved: 800, KeepLast: 1},
		contextTokens:  100,
	})
	got := next.(*chatModel)
	if len(got.messages) != 5 {
		t.Fatalf("compactDone wiped or duplicated the transcript: %+v", got.messages)
	}
	if got.messages[1].content != "old question" || got.messages[2].content != "old answer" {
		t.Fatalf("old transcript was not preserved: %+v", got.messages)
	}
	if !strings.Contains(got.messages[3].content, "Compacted 5 messages") {
		t.Fatalf("missing divider: %+v", got.messages[3])
	}
	if got.messages[4].content != "the summary" {
		t.Fatalf("missing summary: %+v", got.messages[4])
	}
	if len(got.history) != len(newHistory) || got.contextTokens != 100 {
		t.Fatalf("history/context not updated: %+v %d", got.history, got.contextTokens)
	}
}

func TestCompactDoneError(t *testing.T) {
	m := contextTestModel()
	next, _ := m.Update(compactDone{turn: m.turnID, err: errTestCompact})
	got := next.(*chatModel)
	if got.err == nil {
		t.Fatal("compact error not recorded")
	}
	if len(got.messages) == 0 || !strings.Contains(got.messages[len(got.messages)-1].content, "Compact failed") {
		t.Fatalf("missing failure message: %+v", got.messages)
	}
}

var errTestCompact = errors.New("boom")
