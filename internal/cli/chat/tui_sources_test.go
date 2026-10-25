package chat

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func sourcesMessage(m *chatModel) string {
	for i, msg := range m.messages {
		if msg.role == "sources" {
			return m.messages[i].content
		}
	}
	return ""
}

func TestAgentDoneRendersSources(t *testing.T) {
	m := newTestModel(t)
	m.turnID = 4

	result := &types.ChatResponse{Choices: []types.ChatChoice{{Message: types.ChatMessage{
		Role:    "assistant",
		Content: "answer",
		Annotations: []types.ChatAnnotation{
			{URL: "https://a.example", Title: "Title A", SiteName: "Site A"},
			{URL: "https://a.example", Title: "duplicate"},
			{URL: "https://b.example"},
		},
	}}}}

	updated, _ := m.Update(agentDone{
		turn:    4,
		result:  result,
		history: []types.ChatMessage{{Role: "user", Content: "hi"}},
	})
	m = updated.(*chatModel)

	got := sourcesMessage(m)
	if got == "" {
		t.Fatal("agentDone with citations did not append a sources message")
	}
	if !strings.Contains(got, "Sources (2):") {
		t.Errorf("sources = %q, want 2 de-duplicated citations", got)
	}
	if !strings.Contains(got, "Title A") || !strings.Contains(got, "https://b.example") {
		t.Errorf("sources = %q, want both citations rendered", got)
	}
	if strings.Contains(got, "duplicate") {
		t.Errorf("sources = %q, duplicate URL was not removed", got)
	}
}

func TestAgentDoneWithoutSourcesAddsNothing(t *testing.T) {
	m := newTestModel(t)
	m.turnID = 4

	updated, _ := m.Update(agentDone{
		turn:    4,
		result:  &types.ChatResponse{Choices: []types.ChatChoice{{Message: types.ChatMessage{Content: "hi"}}}},
		history: []types.ChatMessage{{Role: "user", Content: "hi"}},
	})
	m = updated.(*chatModel)

	if got := sourcesMessage(m); got != "" {
		t.Errorf("unexpected sources message: %q", got)
	}
}

func TestAgentDoneIgnoresStaleSources(t *testing.T) {
	m := newTestModel(t)
	m.turnID = 5

	updated, _ := m.Update(agentDone{
		turn: 4,
		result: &types.ChatResponse{Choices: []types.ChatChoice{{Message: types.ChatMessage{
			Annotations: []types.ChatAnnotation{{URL: "https://stale.example"}},
		}}}},
		history: []types.ChatMessage{{Role: "user", Content: "stale"}},
	})
	m = updated.(*chatModel)

	if got := sourcesMessage(m); got != "" {
		t.Errorf("stale turn produced sources: %q", got)
	}
}
