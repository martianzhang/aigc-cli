package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestAppendAnnotationUnique(t *testing.T) {
	var got []types.ChatAnnotation
	got = appendAnnotationUnique(got, types.ChatAnnotation{URL: "https://a", Title: "A"})
	got = appendAnnotationUnique(got, types.ChatAnnotation{URL: "https://a", Title: "A (renamed)"}) // same URL
	got = appendAnnotationUnique(got, types.ChatAnnotation{Title: "Title only"})                    // URL-less, kept by title
	got = appendAnnotationUnique(got, types.ChatAnnotation{Title: "Title only"})                    // duplicate title
	got = appendAnnotationUnique(got, types.ChatAnnotation{})                                       // empty, dropped

	if len(got) != 2 {
		t.Fatalf("annotations = %+v, want 2 (URL dedup + title dedup)", got)
	}
	if got[1].Title != "Title only" {
		t.Errorf("second annotation = %+v, want the title-only entry", got[1])
	}
}

// TestChatCompletionStreamAccumulatesAnnotationsAndUsage covers MiMo-style
// streaming: citations arrive in deltas (possibly repeated) and usage is only
// present in the final chunk.
func TestChatCompletionStreamAccumulatesAnnotationsAndUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		write := func(s string) {
			_, _ = io.WriteString(w, s)
			if flusher != nil {
				flusher.Flush()
			}
		}
		write("data: {\"id\":\"c1\",\"model\":\"mimo-v2.6-flash\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"}}]}\n\n")
		write("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ans\",\"annotations\":[{\"type\":\"url_citation\",\"url\":\"https://a.example\",\"title\":\"A\"}]}}]}\n\n")
		// The same URL repeated with a different title must still dedupe.
		write("data: {\"choices\":[{\"index\":0,\"delta\":{\"annotations\":[{\"type\":\"url_citation\",\"url\":\"https://a.example\",\"title\":\"A (dup)\"},{\"type\":\"url_citation\",\"url\":\"https://b.example\",\"title\":\"B\"}]}}]}\n\n")
		write("data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12,\"web_search_usage\":{\"tool_usage\":1,\"page_usage\":5}}}\n\n")
		write("data: [DONE]\n\n")
	}))
	defer srv.Close()

	c := NewWithProvider("sk-test", srv.URL, "", types.ProviderOpenAI)
	res, err := c.ChatCompletion(&types.ChatRequest{
		Model:        "mimo-v2.6-flash",
		Messages:     []types.ChatMessage{{Role: "user", Content: "hi"}},
		Stream:       true,
		OutputWriter: io.Discard,
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	if len(res.Choices) == 0 {
		t.Fatal("no choices returned")
	}
	anns := res.Choices[0].Message.Annotations
	if len(anns) != 2 {
		t.Fatalf("Annotations = %+v, want 2 deduplicated entries", anns)
	}
	if anns[0].URL != "https://a.example" || anns[1].URL != "https://b.example" {
		t.Errorf("Annotations = %+v, want A then B in arrival order", anns)
	}
	if res.Usage == nil || res.Usage.WebSearchUsage == nil {
		t.Fatal("streaming usage was not captured from the final chunk")
	}
	if got := res.Usage.WebSearchUsage.PageUsage; got != 5 {
		t.Errorf("PageUsage = %d, want 5", got)
	}
}
