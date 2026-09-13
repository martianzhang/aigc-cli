package ideas

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/ideas"
)

func TestResolveKeywords_args(t *testing.T) {
	got, err := resolveKeywords([]string{"cinematic", "portrait"})
	if err != nil {
		t.Fatalf("resolveKeywords() returned error: %v", err)
	}
	if got != "cinematic portrait" {
		t.Errorf("resolveKeywords() = %q, want %q", got, "cinematic portrait")
	}
}

func TestResolveKeywords_singleArg(t *testing.T) {
	got, err := resolveKeywords([]string{"portrait"})
	if err != nil {
		t.Fatalf("resolveKeywords() returned error: %v", err)
	}
	if got != "portrait" {
		t.Errorf("resolveKeywords() = %q, want %q", got, "portrait")
	}
}

func TestResolveKeywords_noArgs(t *testing.T) {
	got, err := resolveKeywords(nil)
	if err != nil {
		t.Fatalf("resolveKeywords() returned error: %v", err)
	}
	if got != "" {
		t.Errorf("resolveKeywords() = %q, want empty", got)
	}
}

func captureStdout(fn func()) string {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

func TestOutputMarkdown_multipleResults(t *testing.T) {
	results := []ideas.SearchResult{
		{Entry: ideas.IdeaEntry{Title: "Test One", Prompt: "prompt one", Author: "Alice", License: "MIT"}, Score: 3},
		{Entry: ideas.IdeaEntry{Title: "Test Two", Prompt: "prompt two", Author: "Bob", SourceURL: "https://example.com"}, Score: 1},
	}

	output := captureStdout(func() {
		if err := outputMarkdown(results, "test", 2, nil, false); err != nil {
			t.Errorf("outputMarkdown() returned error: %v", err)
		}
	})

	checks := []struct {
		name string
		want string
	}{
		{"count", "Found 2 result(s)"},
		{"first heading", "## Test One"},
		{"second heading", "## Test Two"},
		{"first prompt", "```\nprompt one\n```"},
		{"second prompt", "```\nprompt two\n```"},
		{"author", "Author: Alice"},
		{"license", "MIT"},
		{"source link", "[Source]"},
		{"separator", "---"},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(output, c.want) {
				t.Errorf("output missing %q", c.want)
			}
		})
	}
}

func TestOutputMarkdown_singleResultNoSeparator(t *testing.T) {
	results := []ideas.SearchResult{
		{Entry: ideas.IdeaEntry{Title: "Only One", Prompt: "single"}, Score: 1},
	}
	output := captureStdout(func() {
		if err := outputMarkdown(results, "test", 1, nil, false); err != nil {
			t.Errorf("outputMarkdown() returned error: %v", err)
		}
	})
	if !strings.Contains(output, "## Only One") {
		t.Errorf("output missing title")
	}
	if strings.Contains(output, "---") {
		t.Errorf("single result should not have separator")
	}
}

func TestOutputMarkdown_zhPrompt(t *testing.T) {
	results := []ideas.SearchResult{
		{Entry: ideas.IdeaEntry{Title: "ZH Test", Prompt: "english prompt", PromptZh: "中文提示词", Lang: "zh"}, Score: 1},
	}
	output := captureStdout(func() {
		if err := outputMarkdown(results, "test", 1, nil, false); err != nil {
			t.Errorf("outputMarkdown() returned error: %v", err)
		}
	})
	if !strings.Contains(output, "中文提示词") {
		t.Errorf("zh entry should show zh prompt, got:\n%s", output)
	}
}

func TestOutputMarkdown_images(t *testing.T) {
	results := []ideas.SearchResult{
		{Entry: ideas.IdeaEntry{Title: "With Img", Prompt: "test", ImageURLs: []string{"https://example.com/img.jpg"}}, Score: 1},
	}
	output := captureStdout(func() {
		if err := outputMarkdown(results, "test", 1, nil, false); err != nil {
			t.Errorf("outputMarkdown() returned error: %v", err)
		}
	})
	if !strings.Contains(output, "![ref]") {
		t.Errorf("output missing image reference")
	}
}

func TestOutputMarkdown_emptyTitle(t *testing.T) {
	results := []ideas.SearchResult{
		{Entry: ideas.IdeaEntry{Prompt: "just a prompt"}, Score: 1},
	}
	output := captureStdout(func() {
		if err := outputMarkdown(results, "test", 1, nil, false); err != nil {
			t.Errorf("outputMarkdown() returned error: %v", err)
		}
	})
	if !strings.Contains(output, "## Result 1") {
		t.Errorf("empty title should fallback to 'Result 1'")
	}
}

func TestOutputJSON(t *testing.T) {
	results := []ideas.SearchResult{
		{Entry: ideas.IdeaEntry{Title: "JSON Test", Prompt: "test prompt"}, Score: 1},
	}
	output := captureStdout(func() {
		if err := outputJSON(results, 1); err != nil {
			t.Errorf("outputJSON() returned error: %v", err)
		}
	})
	var parsed struct {
		Total   int               `json:"total"`
		Results []ideas.IdeaEntry `json:"results"`
	}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, output)
	}
	if parsed.Total != 1 || len(parsed.Results) != 1 || parsed.Results[0].Title != "JSON Test" {
		t.Errorf("unexpected JSON payload: %+v", parsed)
	}
}

func TestLocalImagePath(t *testing.T) {
	if got := localImagePath("", "/tmp/out"); got != "" {
		t.Errorf("localImagePath('') = %q, want empty", got)
	}
	if got := localImagePath("https://example.com/path/to/img.jpg", "/tmp/out"); !strings.HasSuffix(got, "img.jpg") {
		t.Errorf("localImagePath() = %q, should end with img.jpg", got)
	}
}

func TestDefaultConstants(t *testing.T) {
	if defaultLimit != 8 {
		t.Errorf("defaultLimit = %d, want 8", defaultLimit)
	}
}
