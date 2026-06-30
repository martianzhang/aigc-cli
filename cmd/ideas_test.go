package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// --- resolveIdeasKeywords ---

func TestResolveIdeasKeywords_args(t *testing.T) {
	got, err := resolveIdeasKeywords([]string{"cinematic", "portrait"})
	if err != nil {
		t.Fatalf("resolveIdeasKeywords() returned error: %v", err)
	}
	if got != "cinematic portrait" {
		t.Errorf("resolveIdeasKeywords() = %q, want %q", got, "cinematic portrait")
	}
}

func TestResolveIdeasKeywords_singleArg(t *testing.T) {
	got, err := resolveIdeasKeywords([]string{"portrait"})
	if err != nil {
		t.Fatalf("resolveIdeasKeywords() returned error: %v", err)
	}
	if got != "portrait" {
		t.Errorf("resolveIdeasKeywords() = %q, want %q", got, "portrait")
	}
}

func TestResolveIdeasKeywords_noArgs(t *testing.T) {
	got, err := resolveIdeasKeywords(nil)
	if err != nil {
		t.Fatalf("resolveIdeasKeywords() returned error: %v", err)
	}
	if got != "" {
		t.Errorf("resolveIdeasKeywords() = %q, want empty", got)
	}
}

// --- containsWord (word-boundary check) ---

func TestContainsWord_exact(t *testing.T) {
	if !containsWord("cat portrait", "cat") {
		t.Error(`containsWord("cat portrait", "cat") = false, want true`)
	}
}

func TestContainsWord_substring(t *testing.T) {
	if containsWord("category portrait", "cat") {
		t.Error(`containsWord("category portrait", "cat") = true, want false`)
	}
}

func TestContainsWord_absent(t *testing.T) {
	if containsWord("dog portrait", "cat") {
		t.Error(`containsWord("dog portrait", "cat") = true, want false`)
	}
}

func TestContainsWord_endOfString(t *testing.T) {
	if !containsWord("my cat", "cat") {
		t.Error(`containsWord("my cat", "cat") = false, want true`)
	}
}

func TestContainsWord_caseInsensitive(t *testing.T) {
	if !containsWord("MY CAT", "cat") {
		t.Error(`containsWord("MY CAT", "cat") = false, want true`)
	}
}

// --- andFilter ---

func TestAndFilter_allMatch(t *testing.T) {
	// Simulate pre-tokenized docSet
	docSet := map[string]int{"cat": 1, "portrait": 2, "photo": 1}
	if !andFilter(docSet, []string{"cat", "portrait"}) {
		t.Error("andFilter() = false, want true")
	}
}

func TestAndFilter_partialMatch(t *testing.T) {
	docSet := map[string]int{"cat": 1, "photo": 1}
	if andFilter(docSet, []string{"cat", "portrait"}) {
		t.Error("andFilter() = true, want false")
	}
}

func TestAndFilter_zhMatch(t *testing.T) {
	docSet := map[string]int{"电影": 1, "照片": 1}
	if !andFilter(docSet, []string{"电影"}) {
		t.Error("andFilter() should match zh text")
	}
}

// --- tokenize ---

func TestTokenize_basic(t *testing.T) {
	tokens := tokenize("Cat Portrait Photo")
	if len(tokens) != 3 || tokens[0] != "cat" || tokens[1] != "portrait" {
		t.Errorf("tokenize() = %v, want [cat portrait photo]", tokens)
	}
}

func TestTokenize_shortTokensSkipped(t *testing.T) {
	tokens := tokenize("a bc def")
	for _, tok := range tokens {
		if len(tok) < 2 {
			t.Errorf("tokenize() produced short token %q", tok)
		}
	}
}

func TestTokenize_empty(t *testing.T) {
	if tokens := tokenize(""); len(tokens) != 0 {
		t.Errorf("tokenize('') = %v, want empty", tokens)
	}
}

// --- ngramSet ---

func TestNgramSet_basic(t *testing.T) {
	grams := ngramSet("cat", 2)
	if grams["ca"] != 1 || grams["at"] != 1 {
		t.Errorf("ngramSet('cat', 2) = %v, want {ca:1, at:1}", grams)
	}
}

func TestNgramSet_tooShort(t *testing.T) {
	grams := ngramSet("ab", 3)
	if len(grams) != 0 {
		t.Errorf("ngramSet('ab', 3) should be empty, got %v", grams)
	}
}

// --- cosineSimilarity ---

func TestCosineSimilarity_identical(t *testing.T) {
	a := map[string]int{"ca": 1, "at": 1}
	b := map[string]int{"ca": 1, "at": 1}
	sim := cosineSimilarity(a, b)
	if sim < 0.999 || sim > 1.001 {
		t.Errorf("cosineSimilarity(identical) = %f, want ~1.0", sim)
	}
}

func TestCosineSimilarity_orthogonal(t *testing.T) {
	a := map[string]int{"ca": 1}
	b := map[string]int{"do": 1}
	sim := cosineSimilarity(a, b)
	if sim != 0.0 {
		t.Errorf("cosineSimilarity(orthogonal) = %f, want 0.0", sim)
	}
}

// --- searchIdeas (integration via BM25 index) ---

func buildTestIndex(entries []IdeaEntry) *bm25Index {
	return buildBM25Index(entries)
}

func TestSearchIdeas_emptyQuery(t *testing.T) {
	entries := []IdeaEntry{{Title: "Test", Prompt: "test"}}
	idx := buildTestIndex(entries)
	results := searchIdeas(entries, idx, "")
	if results != nil {
		t.Errorf("searchIdeas('') = %v, want nil", results)
	}
}

func TestSearchIdeas_titleMatchRanksHigher(t *testing.T) {
	entries := []IdeaEntry{
		{Title: "Cat Portrait", Prompt: "a cat"},
		{Title: "Something Else", Prompt: "portrait photo"},
	}
	idx := buildTestIndex(entries)
	results := searchIdeas(entries, idx, "portrait")
	if len(results) != 2 {
		t.Fatalf("searchIdeas() = %d results, want 2", len(results))
	}
	// Title match should rank higher
	if results[0].score < results[1].score {
		t.Errorf("title match should rank higher, got %d < %d", results[0].score, results[1].score)
	}
}

func TestSearchIdeas_noMatches(t *testing.T) {
	entries := []IdeaEntry{{Title: "Cat", Prompt: "meow"}}
	idx := buildTestIndex(entries)
	results := searchIdeas(entries, idx, "portrait")
	if len(results) != 0 {
		t.Errorf("searchIdeas() = %d results, want 0", len(results))
	}
}

func TestSearchIdeas_andSemantics(t *testing.T) {
	entries := []IdeaEntry{
		{Title: "Cat and Dog", Prompt: "cat dog"},
		{Title: "Only Cat", Prompt: "just a cat"},
	}
	idx := buildTestIndex(entries)
	results := searchIdeas(entries, idx, "cat dog")
	if len(results) != 1 {
		t.Errorf("searchIdeas('cat dog') = %d results, want 1 (only first has both)", len(results))
	}
}

func TestSearchIdeas_caseInsensitive(t *testing.T) {
	entries := []IdeaEntry{{Title: "CINEMATIC PORTRAIT", Prompt: "test"}}
	idx := buildTestIndex(entries)
	results := searchIdeas(entries, idx, "cinematic")
	if len(results) == 0 {
		t.Error("searchIdeas('cinematic') = 0, want match (case insensitive)")
	}
}

func TestSearchIdeas_zhMatch(t *testing.T) {
	entries := []IdeaEntry{{TitleZh: "电影感肖像", PromptZh: "一张电影感肖像照片"}}
	idx := buildTestIndex(entries)
	results := searchIdeas(entries, idx, "电影")
	if len(results) == 0 {
		t.Error("searchIdeas('电影') = 0, want match for zh text")
	}
}

// --- outputMarkdown ---

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
	results := []searchResult{
		{entry: IdeaEntry{Title: "Test One", Prompt: "prompt one", Author: "Alice", License: "MIT"}, score: 3},
		{entry: IdeaEntry{Title: "Test Two", Prompt: "prompt two", Author: "Bob", SourceURL: "https://example.com"}, score: 1},
	}

	output := captureStdout(func() {
		if err := outputMarkdown(results, "test", 2, nil); err != nil {
			t.Errorf("outputMarkdown() returned error: %v", err)
		}
	})

	checks := []struct {
		name string
		want string
	}{
		{"title", "# Ideas: test"},
		{"count", "找到 2 个结果"},
		{"first heading", "## Test One"},
		{"second heading", "## Test Two"},
		{"first prompt", "```\nprompt one\n```"},
		{"second prompt", "```\nprompt two\n```"},
		{"author", "作者: Alice"},
		{"license", "MIT"},
		{"source link", "[来源]"},
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
	results := []searchResult{
		{entry: IdeaEntry{Title: "Only One", Prompt: "single"}, score: 1},
	}
	output := captureStdout(func() {
		if err := outputMarkdown(results, "test", 1, nil); err != nil {
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
	results := []searchResult{
		{entry: IdeaEntry{Title: "ZH Test", Prompt: "english prompt", PromptZh: "中文提示词", Lang: "zh"}, score: 1},
	}
	output := captureStdout(func() {
		if err := outputMarkdown(results, "test", 1, nil); err != nil {
			t.Errorf("outputMarkdown() returned error: %v", err)
		}
	})
	if !strings.Contains(output, "中文提示词") {
		t.Errorf("zh entry should show zh prompt, got:\n%s", output)
	}
}

func TestOutputMarkdown_images(t *testing.T) {
	results := []searchResult{
		{entry: IdeaEntry{Title: "With Img", Prompt: "test", ImageURLs: []string{"https://example.com/img.jpg"}}, score: 1},
	}
	output := captureStdout(func() {
		if err := outputMarkdown(results, "test", 1, nil); err != nil {
			t.Errorf("outputMarkdown() returned error: %v", err)
		}
	})
	if !strings.Contains(output, "![参考图 1]") {
		t.Errorf("output missing image reference")
	}
}

func TestOutputMarkdown_emptyTitle(t *testing.T) {
	results := []searchResult{
		{entry: IdeaEntry{Prompt: "just a prompt"}, score: 1},
	}
	output := captureStdout(func() {
		if err := outputMarkdown(results, "test", 1, nil); err != nil {
			t.Errorf("outputMarkdown() returned error: %v", err)
		}
	})
	if !strings.Contains(output, "## Result 1") {
		t.Errorf("empty title should fallback to 'Result 1'")
	}
}

// --- outputJSON ---

func TestOutputJSON(t *testing.T) {
	results := []searchResult{
		{entry: IdeaEntry{Title: "JSON Test", Prompt: "test prompt"}, score: 1},
	}
	output := captureStdout(func() {
		if err := outputJSON(results, 1); err != nil {
			t.Errorf("outputJSON() returned error: %v", err)
		}
	})
	var parsed struct {
		Total   int         `json:"total"`
		Results []IdeaEntry `json:"results"`
	}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, output)
	}
	if parsed.Total != 1 {
		t.Errorf("total = %d, want 1", parsed.Total)
	}
	if len(parsed.Results) != 1 {
		t.Errorf("results = %d, want 1", len(parsed.Results))
	}
	if parsed.Results[0].Title != "JSON Test" {
		t.Errorf("title = %q", parsed.Results[0].Title)
	}
}

// --- localImagePath ---

func TestLocalImagePath_empty(t *testing.T) {
	if got := localImagePath(""); got != "" {
		t.Errorf("localImagePath('') = %q, want empty", got)
	}
}

func TestLocalImagePath_fullURL(t *testing.T) {
	got := localImagePath("https://example.com/path/to/img.jpg")
	if !strings.HasSuffix(got, "img.jpg") {
		t.Errorf("localImagePath() = %q, should end with img.jpg", got)
	}
}

// --- default constants ---

func TestDefaultConstants(t *testing.T) {
	if ideasDefaultLimit != 8 {
		t.Errorf("ideasDefaultLimit = %d, want 8", ideasDefaultLimit)
	}
}
