package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestEstimateTokens(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"ascii", "hello", 2},
		{"ascii 8", "abcdefgh", 2},
		{"cjk", "你好世界", 4},
		{"mixed", "hi你好", 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EstimateTokens(c.in); got != c.want {
				t.Errorf("EstimateTokens(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

func TestEstimateMessageCountsToolArguments(t *testing.T) {
	args := `{"path":"a.go","content":"` + strings.Repeat("x", 20000) + `"}`
	msg := types.ChatMessage{
		Role:      "assistant",
		ToolCalls: []types.ToolCall{{ID: "call_1", Function: types.ToolCallFunction{Name: "write", Arguments: args}}},
	}
	got := EstimateMessageTokens(msg)
	if got < 4000 {
		t.Fatalf("EstimateMessageTokens() = %d, want tool arguments counted (>=4000)", got)
	}
}

func TestEstimateImageUsesFlatValue(t *testing.T) {
	img := types.ChatImage{Name: "big.png", MIMEType: "image/png", Data: make([]byte, 1<<20)}
	if got := EstimateImageTokens(img); got != defaultImageTokens {
		t.Fatalf("EstimateImageTokens() = %d, want %d", got, defaultImageTokens)
	}
}

func TestEstimateContextUsesUsageAnchor(t *testing.T) {
	history := []types.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	anchor := &UsageAnchor{MessageIndex: 2, PromptTokens: 1000}
	got := EstimateContext(history, anchor, 50)
	want := 1000 + EstimateMessageTokens(history[2])
	if got != want {
		t.Fatalf("EstimateContext() = %d, want %d", got, want)
	}
	if got := AnalyzeContext(history, anchor, 50); !got.AnchorUsed || got.UsagePart != 1000 {
		t.Fatalf("AnalyzeContext() anchor = %+v", got)
	}
}

func TestAnalyzeContextBreakdown(t *testing.T) {
	history := []types.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "ok"},
		{Role: "tool", Content: "result"},
	}
	got := AnalyzeContext(history, nil, 25)
	if got.Breakdown.Tools != 25 || got.Breakdown.System == 0 || got.Breakdown.User == 0 ||
		got.Breakdown.Assistant == 0 || got.Breakdown.ToolResults == 0 {
		t.Fatalf("AnalyzeContext() breakdown = %+v", got.Breakdown)
	}
	if got.Breakdown.Total != got.Breakdown.System+got.Breakdown.Tools+got.Breakdown.User+
		got.Breakdown.Assistant+got.Breakdown.ToolResults {
		t.Fatalf("breakdown total does not add up: %+v", got.Breakdown)
	}
}

func TestFindCutPointKeepsTailWithoutOrphanTool(t *testing.T) {
	messages := []types.ChatMessage{
		{Role: "user", Content: strings.Repeat("old context ", 40)},
		{Role: "assistant", Content: strings.Repeat("old answer ", 40)},
		{Role: "user", Content: "recent question"},
		{Role: "assistant", ToolCalls: []types.ToolCall{{ID: "call_1", Function: types.ToolCallFunction{Name: "read"}}}},
		{Role: "tool", ToolCallID: "call_1", Content: strings.Repeat("recent tool output ", 20)},
		{Role: "assistant", Content: "recent answer"},
	}
	cut := findCutPoint(messages, 20)
	if cut.firstKept <= 0 || cut.keepLast <= 0 || cut.keepLast >= len(messages) {
		t.Fatalf("findCutPoint() = %+v, want a middle cut", cut)
	}
	tail := messages[cut.firstKept:]
	if !validCutTail(tail) {
		t.Fatalf("tail has an orphan tool message: %+v", tail)
	}
}

func TestFindCutPointNothingToCompact(t *testing.T) {
	messages := []types.ChatMessage{{Role: "user", Content: "only message"}}
	if cut := findCutPoint(messages, 10); cut.firstKept != 0 {
		t.Fatalf("findCutPoint() = %+v, want no-op", cut)
	}
}

func TestValidCutTail(t *testing.T) {
	tail := []types.ChatMessage{
		{Role: "assistant", ToolCalls: []types.ToolCall{{ID: "a"}}},
		{Role: "tool", ToolCallID: "a"},
	}
	if !validCutTail(tail) {
		t.Fatal("validCutTail() = false, want true")
	}
	if validCutTail(tail[1:]) {
		t.Fatal("validCutTail() = true for an orphan tool message")
	}
}

func TestSerializeConversationTruncatesAndDropsImages(t *testing.T) {
	msg := types.ChatMessage{
		Role:    "user",
		Content: "look at this",
		Images:  []types.ChatImage{{Name: "cat.png", Data: []byte("base64-data")}},
	}
	tool := types.ChatMessage{Role: "tool", Content: strings.Repeat("x", 5000)}
	assistant := types.ChatMessage{
		Role:      "assistant",
		ToolCalls: []types.ToolCall{{Function: types.ToolCallFunction{Name: "write", Arguments: strings.Repeat("y", 5000)}}},
	}
	out := serializeConversation([]types.ChatMessage{msg, assistant, tool}, 200, 100)
	if strings.Contains(out, "base64-data") {
		t.Fatal("serialized summary contains image bytes")
	}
	if !strings.Contains(out, "[Image]: cat.png") {
		t.Fatalf("serialized summary missing image placeholder: %s", out)
	}
	if len(out) > 2000 {
		t.Fatalf("serialized summary is too large: %d", len(out))
	}
	if !strings.Contains(out, "more bytes truncated") {
		t.Fatalf("serialized summary missing truncation marker: %s", out)
	}
}

func TestFileOps(t *testing.T) {
	ops := newFileOps()
	ops.addPrevious([]string{"old-read.go"}, []string{"old-edit.go"})
	ops.addMessages([]types.ChatMessage{
		{Role: "assistant", ToolCalls: []types.ToolCall{
			{Function: types.ToolCallFunction{Name: "read", Arguments: `{"path":"a.go"}`}},
			{Function: types.ToolCallFunction{Name: "write", Arguments: `{"path":"b.go"}`}},
			{Function: types.ToolCallFunction{Name: "edit", Arguments: `{"path":"c.go"}`}},
		}},
	})
	read, modified := ops.lists()
	if strings.Join(read, ",") != "a.go,old-read.go" {
		t.Fatalf("read = %v", read)
	}
	if strings.Join(modified, ",") != "b.go,c.go,old-edit.go" {
		t.Fatalf("modified = %v", modified)
	}
	formatted := formatFileOperations(read, modified)
	if !strings.Contains(formatted, "<read-files>") || !strings.Contains(formatted, "<modified-files>") {
		t.Fatalf("formatFileOperations() = %q", formatted)
	}
}

func TestStripFileOperationBlocks(t *testing.T) {
	in := "summary\n\n<read-files>\na.go\n</read-files>\n\n<modified-files>\nb.go\n</modified-files>"
	got := stripFileOperationBlocks(in)
	if strings.Contains(got, "<read-files>") || strings.Contains(got, "<modified-files>") {
		t.Fatalf("stripFileOperationBlocks() = %q", got)
	}
	if !strings.Contains(got, "summary") {
		t.Fatalf("stripFileOperationBlocks() removed the summary: %q", got)
	}
}

func TestBuildSummaryPromptModes(t *testing.T) {
	initial := buildSummaryPrompt("conv", "", "", false)
	if !strings.Contains(initial, "## Goal") || strings.Contains(initial, "<previous-summary>") {
		t.Fatalf("initial prompt = %q", initial)
	}
	update := buildSummaryPrompt("conv", "old summary", "focus on auth", false)
	if !strings.Contains(update, "<previous-summary>") || !strings.Contains(update, "PRESERVE") {
		t.Fatalf("update prompt = %q", update)
	}
	if !strings.Contains(update, "focus on auth") {
		t.Fatalf("custom focus missing: %q", update)
	}
	split := buildSummaryPrompt("conv", "", "", true)
	if !strings.Contains(split, "ongoing turn") {
		t.Fatalf("split note missing: %q", split)
	}
}

func compactSettings(window int) Settings {
	return Settings{Enabled: true, Window: window, ReserveTokens: 200, KeepRecentTokens: 200}
}

func summaryServer(t *testing.T, body *string, status int, finish string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if body != nil {
			*body = string(raw)
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"message":"server error"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"## Goal\nfinish the task"},"finish_reason":"` + finish + `"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
}

func testClient(t *testing.T, url string) *client.Client {
	t.Helper()
	return client.NewWithProvider("sk-test", url, "", types.ProviderOpenAI)
}

func summaryPromptFromBody(t *testing.T, body string) string {
	t.Helper()
	var req types.ChatRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode summary request: %v", err)
	}
	var b strings.Builder
	for _, m := range req.Messages {
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	return b.String()
}

func longHistory() []types.ChatMessage {
	history := []types.ChatMessage{{Role: "system", Content: "system prompt"}}
	for i := 0; i < 12; i++ {
		history = append(history,
			types.ChatMessage{Role: "user", Content: strings.Repeat("question ", 20)},
			types.ChatMessage{Role: "assistant", Content: strings.Repeat("answer ", 20)},
		)
	}
	return history
}

func TestCompactKeepsRecentTailAndPersistsSummary(t *testing.T) {
	var body string
	srv := summaryServer(t, &body, http.StatusOK, "stop")
	defer srv.Close()
	history := longHistory()

	res, err := Compact(context.Background(), testClient(t, srv.URL), history, CompactOptions{
		Model:    "test-model",
		Settings: compactSettings(1200),
		Stderr:   io.Discard,
		Retry:    RetryPolicy{MaxAttempts: 1},
	})
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if res.KeepLast <= 0 || res.KeepLast >= len(history) {
		t.Fatalf("KeepLast = %d, want a middle tail", res.KeepLast)
	}
	if res.Tokens >= res.TokensBefore || res.TokenSaved <= 0 {
		t.Fatalf("tokens before/after = %d/%d", res.TokensBefore, res.Tokens)
	}
	// New history = system + summary + tail.
	if len(res.History) != 2+res.KeepLast {
		t.Fatalf("history length = %d, want %d", len(res.History), 2+res.KeepLast)
	}
	if res.History[0].Role != "system" || !strings.HasPrefix(res.History[1].Content, PreviousSummaryPrefix) {
		t.Fatalf("history does not start with system + summary: %+v", res.History[:2])
	}
	tail := res.History[len(res.History)-res.KeepLast:]
	for i := range tail {
		if tail[i].Role != history[len(history)-res.KeepLast+i].Role {
			t.Fatalf("tail message %d does not match original", i)
		}
	}
	prompt := summaryPromptFromBody(t, body)
	if !strings.Contains(prompt, "<conversation>") {
		t.Fatalf("summary request is not serialized text: %s", prompt)
	}
	if strings.Contains(prompt, "system prompt") {
		t.Fatalf("summary request should not replay the system prompt: %s", prompt)
	}
	if strings.Contains(prompt, "base64") {
		t.Fatalf("summary request contains image data: %s", prompt)
	}
}

func TestCompactUsesPreviousSummary(t *testing.T) {
	var body string
	srv := summaryServer(t, &body, http.StatusOK, "stop")
	defer srv.Close()
	history := append([]types.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "system", Content: PreviousSummaryPrefix + "old checkpoint"},
	}, longHistory()[1:]...)
	res, err := Compact(context.Background(), testClient(t, srv.URL), history, CompactOptions{
		Model:    "m",
		Settings: compactSettings(1200),
		Stderr:   io.Discard,
		Retry:    RetryPolicy{MaxAttempts: 1},
	})
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	prompt := summaryPromptFromBody(t, body)
	if !strings.Contains(prompt, "<previous-summary>") || !strings.Contains(prompt, "old checkpoint") {
		t.Fatalf("previous summary not sent: %s", prompt)
	}
	if res.History[0].Role != "system" || res.History[0].Content != "sys" {
		t.Fatalf("original system prompt was not preserved: %+v", res.History[0])
	}
	if !strings.HasPrefix(res.History[1].Content, PreviousSummaryPrefix) || !strings.Contains(res.History[1].Content, "finish the task") {
		t.Fatalf("new summary was not inserted: %+v", res.History[1])
	}
}

func TestCompactManualShortHistoryStillCompacts(t *testing.T) {
	srv := summaryServer(t, nil, http.StatusOK, "stop")
	defer srv.Close()
	history := []types.ChatMessage{{Role: "system", Content: "sys"}}
	for i := 0; i < 8; i++ {
		history = append(history,
			types.ChatMessage{Role: "user", Content: strings.Repeat("question ", 20)},
			types.ChatMessage{Role: "assistant", Content: strings.Repeat("answer ", 20)},
		)
	}
	res, err := Compact(context.Background(), testClient(t, srv.URL), history, CompactOptions{
		Model:    "m",
		Settings: Settings{Window: 0, KeepRecentTokens: 20000},
		Force:    true,
		Stderr:   io.Discard,
		Retry:    RetryPolicy{MaxAttempts: 1},
	})
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if res.KeepLast == 0 {
		t.Fatalf("manual compaction kept no tail: %+v", res)
	}
	if len(res.History) != 2+res.KeepLast {
		t.Fatalf("history length = %d, want %d", len(res.History), 2+res.KeepLast)
	}
}

func TestCompactRejectsTruncatedSummary(t *testing.T) {
	srv := summaryServer(t, nil, http.StatusOK, "length")
	defer srv.Close()
	_, err := Compact(context.Background(), testClient(t, srv.URL), longHistory(), CompactOptions{
		Model: "m", Settings: compactSettings(1200), Stderr: io.Discard,
		Retry: RetryPolicy{MaxAttempts: 1},
	})
	if err == nil || !strings.Contains(err.Error(), "token cap") {
		t.Fatalf("Compact() error = %v, want a truncation error", err)
	}
}

func TestCompactRetriesWhenSummaryIsTooLarge(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		content := "## Goal\nok"
		if calls == 1 {
			content = strings.Repeat("verbose summary ", 500)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": content}, "finish_reason": "stop"}},
		})
	}))
	defer srv.Close()
	res, err := Compact(context.Background(), testClient(t, srv.URL), longHistory(), CompactOptions{
		Model: "m", Settings: compactSettings(1200), Stderr: io.Discard,
		Retry: RetryPolicy{MaxAttempts: 1},
	})
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("summary calls = %d, want 2", calls)
	}
	if !strings.Contains(res.Summary, "## Goal") || strings.Contains(res.Summary, "verbose summary verbose") {
		t.Fatalf("summary = %q, want the concise retry", res.Summary)
	}
}

func TestCompactRetriesTransient(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"## Goal\nok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()
	_, err := Compact(context.Background(), testClient(t, srv.URL), longHistory(), CompactOptions{
		Model: "m", Settings: compactSettings(1200), Stderr: io.Discard,
		Retry: RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond},
	})
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestAutoCompactBelowThreshold(t *testing.T) {
	res, err := AutoCompact(context.Background(), nil, longHistory(), CompactOptions{
		Settings: Settings{Enabled: true, Window: 1_000_000},
		Stderr:   io.Discard,
	})
	if err != nil || res != nil {
		t.Fatalf("AutoCompact() = %+v, %v; want nil, nil", res, err)
	}
}

func TestAutoCompactDisabled(t *testing.T) {
	res, err := AutoCompact(context.Background(), nil, longHistory(), CompactOptions{
		Settings: Settings{Enabled: false, Window: 100},
		Stderr:   io.Discard,
	})
	if err != nil || res != nil {
		t.Fatalf("AutoCompact() = %+v, %v; want nil, nil", res, err)
	}
}

func TestCompactNothingToCompact(t *testing.T) {
	_, err := Compact(context.Background(), testClient(t, "http://127.0.0.1:1"), []types.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "only request"},
	}, CompactOptions{Model: "m", Settings: compactSettings(100)})
	if err != ErrNothingToCompact {
		t.Fatalf("Compact() error = %v, want ErrNothingToCompact", err)
	}
}

func TestEmergencyTrimDropsOldTurns(t *testing.T) {
	history := longHistory()
	out, keepLast, dropped, ok := EmergencyTrim(history, Settings{
		Enabled: true, Window: 300, ReserveTokens: 100, KeepRecentTokens: 100,
	}, 0)
	if !ok || dropped == 0 || keepLast <= 0 {
		t.Fatalf("EmergencyTrim() = ok=%v dropped=%d keepLast=%d", ok, dropped, keepLast)
	}
	if !validCutTail(out[2:]) {
		t.Fatalf("emergency history has an orphan tool message: %+v", out)
	}
	if EstimateContext(out, nil, 0) > (Settings{Window: 300, ReserveTokens: 100}).Threshold() {
		t.Fatalf("emergency history still exceeds the budget")
	}
}

func TestIsContextOverflow(t *testing.T) {
	cases := []struct {
		err  string
		want bool
	}{
		{"API returned status 400: maximum context length exceeded", true},
		{"context_length_exceeded", true},
		{"reduce the length of the messages", true},
		{"API returned status 401: invalid api key", false},
		{"network unreachable", false},
	}
	for _, c := range cases {
		if got := IsContextOverflow(ioError(c.err)); got != c.want {
			t.Errorf("IsContextOverflow(%q) = %v, want %v", c.err, got, c.want)
		}
	}
}

type stringError string

func (e stringError) Error() string { return string(e) }
func ioError(s string) error        { return stringError(s) }
