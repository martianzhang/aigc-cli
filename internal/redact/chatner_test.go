package redact

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

type fakeChat struct {
	content string
	err     error
}

func (f *fakeChat) ChatCompletion(req *types.ChatRequest) (*types.ChatResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &types.ChatResponse{Choices: []types.ChatChoice{{
		Message: types.ChatMessage{Content: f.content},
	}}}, nil
}

func TestChatnerAnchorsAmbiguousOccurrences(t *testing.T) {
	fc := &fakeChat{content: `{"人名":["高峰"],"电话":["13812345678"]}`}
	eng, err := NewChatner(fc, "has4", nil)
	if err != nil {
		t.Fatal(err)
	}
	text := "联系人：高峰，电话13812345678。高峰时段车流量较大。"
	result, err := eng.Detect(context.Background(), NewDocument("t", text))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Spans) != 2 {
		t.Fatalf("spans = %+v, want PERSON + PHONE", result.Spans)
	}
	if len(result.Review) != 1 || result.Review[0].Text != "高峰" {
		t.Fatalf("review = %+v, want the unanchored 高峰", result.Review)
	}
}

func TestChatnerOccurrenceAll(t *testing.T) {
	fc := &fakeChat{content: "```json\n{\"人名\":[\"高峰\"]}\n```"}
	eng, err := NewChatner(fc, "has4", &types.RedactChatnerConfig{Occurrence: OccurrenceAll})
	if err != nil {
		t.Fatal(err)
	}
	text := "高峰时段与联系人高峰"
	result, err := eng.Detect(context.Background(), NewDocument("t", text))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Spans) != 2 || len(result.Review) != 0 {
		t.Fatalf("spans=%+v review=%+v", result.Spans, result.Review)
	}
}

func TestChatnerErrorAndEmpty(t *testing.T) {
	eng, _ := NewChatner(&fakeChat{err: errors.New("boom")}, "has4", nil)
	if _, err := eng.Detect(context.Background(), NewDocument("t", "x")); err == nil {
		t.Fatal("expected error")
	}
	eng, _ = NewChatner(&fakeChat{content: "not json"}, "has4", nil)
	result, err := eng.Detect(context.Background(), NewDocument("t", "x"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Spans) != 0 {
		t.Fatalf("expected no spans, got %+v", result.Spans)
	}
}

func TestChunkTextCoversInput(t *testing.T) {
	text := strings.Repeat("字", 7000) + "电话13812345678。"
	chunks := chunkText(text, 3000, 200)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	if chunks[0].Start != 0 {
		t.Fatalf("first chunk start = %d", chunks[0].Start)
	}
	for i := 1; i < len(chunks); i++ {
		if chunks[i].Start <= chunks[i-1].Start {
			t.Fatalf("chunk %d start not increasing", i)
		}
	}
	// Every phone occurrence must live in at least one chunk.
	found := false
	for _, c := range chunks {
		if strings.Contains(c.Text, "13812345678") {
			found = true
		}
	}
	if !found {
		t.Fatal("phone not covered by any chunk")
	}
}

func TestChatnerChunksLongDocument(t *testing.T) {
	fc := &fakeChat{content: `{"电话":["13812345678"]}`}
	eng, _ := NewChatner(fc, "has4", nil)
	text := strings.Repeat("无关内容。", 1200) + "联系电话13812345678。"
	result, err := eng.Detect(context.Background(), NewDocument("t", text))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Spans) != 1 || result.Spans[0].Text != "13812345678" {
		t.Fatalf("chunked detection failed: %+v", result.Spans)
	}
}

func TestChatnerRejectsBadConfig(t *testing.T) {
	if _, err := NewChatner(nil, "x", nil); err == nil {
		t.Fatal("expected nil client error")
	}
	if _, err := NewChatner(&fakeChat{}, "", nil); err == nil {
		t.Fatal("expected empty model error")
	}
	if _, err := NewChatner(&fakeChat{}, "x", &types.RedactChatnerConfig{Occurrence: "nope"}); err == nil {
		t.Fatal("expected bad occurrence error")
	}
}
