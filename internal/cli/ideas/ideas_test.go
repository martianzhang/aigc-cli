package ideas

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/ideas"
	"github.com/martianzhang/aigc-cli/internal/types"
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

func TestResolveKeywords_helloWorld(t *testing.T) {
	got, err := resolveKeywords([]string{"hello", "world"})
	if err != nil {
		t.Fatalf("resolveKeywords() returned error: %v", err)
	}
	if got != "hello world" {
		t.Errorf("resolveKeywords() = %q, want %q", got, "hello world")
	}
}

// withTempHome points the user home directory (Unix $HOME, Windows
// %USERPROFILE%) at a fresh temp dir so ideasDir() stays inside the test.
func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func createDefaultIdeasFile(t *testing.T, home string) string {
	t.Helper()
	path := filepath.Join(home, ".config", "aigc-cli", "ideas", "ideas.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("cannot create default ideas dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("[]"), 0o644); err != nil {
		t.Fatalf("cannot write default ideas file: %v", err)
	}
	return path
}

func TestResolveDataPath_configOverride(t *testing.T) {
	withTempHome(t)
	cfg := &types.Config{Ideas: &types.IdeasConfig{DataPath: "/custom/ideas.json"}}
	if got := resolveDataPath(cfg); got != "/custom/ideas.json" {
		t.Errorf("resolveDataPath() = %q, want %q", got, "/custom/ideas.json")
	}
}

func TestResolveDataPath_nilConfigMissingFile(t *testing.T) {
	withTempHome(t)
	if got := resolveDataPath(nil); got != "" {
		t.Errorf("resolveDataPath(nil) = %q, want empty when default file is missing", got)
	}
}

func TestResolveDataPath_nilIdeasMissingFile(t *testing.T) {
	withTempHome(t)
	if got := resolveDataPath(&types.Config{}); got != "" {
		t.Errorf("resolveDataPath(cfg) = %q, want empty when cfg.Ideas is nil and default file is missing", got)
	}
}

func TestResolveDataPath_defaultFileExists(t *testing.T) {
	home := withTempHome(t)
	want := createDefaultIdeasFile(t, home)
	if got := resolveDataPath(nil); got != want {
		t.Errorf("resolveDataPath(nil) = %q, want %q", got, want)
	}
	if got := resolveDataPath(&types.Config{}); got != want {
		t.Errorf("resolveDataPath(cfg) = %q, want %q", got, want)
	}
}

func TestDataSavePath_configOverride(t *testing.T) {
	withTempHome(t)
	cfg := &types.Config{Ideas: &types.IdeasConfig{DataPath: "/custom/ideas.json"}}
	if got := dataSavePath(cfg); got != "/custom/ideas.json" {
		t.Errorf("dataSavePath() = %q, want %q", got, "/custom/ideas.json")
	}
}

func TestDataSavePath_defaultPathWithoutFile(t *testing.T) {
	home := withTempHome(t)
	want := filepath.Join(home, ".config", "aigc-cli", "ideas", "ideas.json")
	if got := dataSavePath(nil); got != want {
		t.Errorf("dataSavePath(nil) = %q, want %q", got, want)
	}
	if got := dataSavePath(&types.Config{}); got != want {
		t.Errorf("dataSavePath(cfg) = %q, want %q", got, want)
	}
	// dataSavePath is the download target: it must not require the file to exist,
	// unlike resolveDataPath which returns "" while the file is missing.
	if _, err := os.Stat(want); err == nil {
		t.Fatalf("precondition failed: %s should not exist", want)
	}
	if got := resolveDataPath(nil); got != "" {
		t.Errorf("resolveDataPath(nil) = %q, want empty while file is missing", got)
	}
}

func TestOutputJSON_multipleResults(t *testing.T) {
	results := []ideas.SearchResult{
		{Entry: ideas.IdeaEntry{Title: "First", Prompt: "prompt one", Author: "Alice"}, Score: 9},
		{Entry: ideas.IdeaEntry{Title: "Second", Prompt: "prompt two", Author: "Bob"}, Score: 4},
	}
	var outErr error
	output := captureStdout(func() { outErr = outputJSON(results, 7) })
	if outErr != nil {
		t.Fatalf("outputJSON() returned error: %v", outErr)
	}

	var parsed struct {
		Total   int               `json:"total"`
		Results []ideas.IdeaEntry `json:"results"`
	}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, output)
	}
	if parsed.Total != 7 {
		t.Errorf("total = %d, want 7 (independent of len(results))", parsed.Total)
	}
	if len(parsed.Results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(parsed.Results))
	}
	if parsed.Results[0].Title != "First" || parsed.Results[0].Prompt != "prompt one" {
		t.Errorf("results[0] = %+v, want First/prompt one", parsed.Results[0])
	}
	if parsed.Results[1].Title != "Second" || parsed.Results[1].Author != "Bob" {
		t.Errorf("results[1] = %+v, want Second/Bob", parsed.Results[1])
	}
	if !strings.HasPrefix(output, "{\n") {
		t.Errorf("expected indented JSON object, got:\n%s", output)
	}
}

func TestOutputJSON_emptyResults(t *testing.T) {
	var outErr error
	output := captureStdout(func() { outErr = outputJSON(nil, 0) })
	if outErr != nil {
		t.Fatalf("outputJSON() returned error: %v", outErr)
	}

	var parsed struct {
		Total   int               `json:"total"`
		Results []ideas.IdeaEntry `json:"results"`
	}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, output)
	}
	if parsed.Total != 0 || len(parsed.Results) != 0 {
		t.Errorf("unexpected payload: %+v", parsed)
	}
}
