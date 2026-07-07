package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/apimart-cli/internal/types"
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
	if !strings.Contains(output, "![ref]") {
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

// --- cache tests ---

func TestComputeHash_deterministic(t *testing.T) {
	h1 := computeHash([]byte("hello"))
	h2 := computeHash([]byte("hello"))
	if len(h1) != 32 {
		t.Errorf("hash length = %d, want 32", len(h1))
	}
	if !bytes.Equal(h1, h2) {
		t.Error("computeHash not deterministic")
	}
}

func TestComputeHash_different(t *testing.T) {
	h1 := computeHash([]byte("hello"))
	h2 := computeHash([]byte("world"))
	if bytes.Equal(h1, h2) {
		t.Error("different inputs should produce different hashes")
	}
}

func TestSaveLoadCachedIndex_roundTrip(t *testing.T) {
	dir := t.TempDir()

	// Create a minimal config with cache enabled pointing to temp dir
	cfg := &types.Config{
		Ideas: &types.IdeasConfig{
			DataPath:     filepath.Join(dir, "ideas.json"),
			IndexPath:    filepath.Join(dir, "ideas.index"),
			CacheEnabled: true,
		},
	}

	// Build a real index from test entries
	entries := []IdeaEntry{
		{Title: "Test", Prompt: "a test prompt", Lang: "en"},
		{Title: "Cat Photo", Prompt: "a cat photo", Lang: "en"},
	}
	idx := buildBM25Index(entries)

	// Also save a fake ideas.json so the hash can be validated
	jsonData, _ := json.Marshal(entries)
	os.WriteFile(cfg.Ideas.DataPath, jsonData, 0644)

	hash := computeHash(jsonData)

	// Save
	saveCachedIndex(cfg, idx, hash)

	// Load
	loaded := loadCachedIndex(cfg, hash)
	if loaded == nil {
		t.Fatal("loadCachedIndex returned nil after save")
	}

	// Verify fields
	if loaded.avgDocLen != idx.avgDocLen {
		t.Errorf("avgDocLen mismatch: %f vs %f", loaded.avgDocLen, idx.avgDocLen)
	}
	if loaded.docCount != idx.docCount {
		t.Errorf("docCount mismatch: %d vs %d", loaded.docCount, idx.docCount)
	}
	if len(loaded.idf) != len(idx.idf) {
		t.Errorf("idf size mismatch: %d vs %d", len(loaded.idf), len(idx.idf))
	}
	if len(loaded.docTokens) != len(idx.docTokens) {
		t.Errorf("docTokens size mismatch: %d vs %d", len(loaded.docTokens), len(idx.docTokens))
	}
	if len(loaded.docSet) != len(idx.docSet) {
		t.Errorf("docSet size mismatch: %d vs %d", len(loaded.docSet), len(idx.docSet))
	}

	// Verify search still works with loaded index
	results := searchIdeas(entries, loaded, "cat")
	if len(results) == 0 {
		t.Error("searchIdeas with cached index returned no results")
	}
}

func TestLoadCachedIndex_hashMismatch(t *testing.T) {
	dir := t.TempDir()
	cfg := &types.Config{
		Ideas: &types.IdeasConfig{
			DataPath:     filepath.Join(dir, "ideas.json"),
			IndexPath:    filepath.Join(dir, "ideas.index"),
			CacheEnabled: true,
		},
	}

	entries := []IdeaEntry{{Title: "Test", Prompt: "test"}}
	idx := buildBM25Index(entries)
	jsonData, _ := json.Marshal(entries)
	os.WriteFile(cfg.Ideas.DataPath, jsonData, 0644)

	// Save with one hash
	originalHash := computeHash(jsonData)
	saveCachedIndex(cfg, idx, originalHash)

	// Make ideas.json content different (simulate data update)
	newData := []byte(`[{"title":"Updated","prompt":"different data","lang":"en"}]`)
	os.WriteFile(cfg.Ideas.DataPath, newData, 0644)
	newHash := computeHash(newData)

	// Load with new hash — should fail because old cache has originalHash
	loaded := loadCachedIndex(cfg, newHash)
	if loaded != nil {
		t.Error("loadCachedIndex should return nil on hash mismatch")
	}
}

func TestLoadCachedIndex_disabledCache(t *testing.T) {
	cfg := &types.Config{
		Ideas: &types.IdeasConfig{
			CacheEnabled: false,
		},
	}
	loaded := loadCachedIndex(cfg, []byte{1, 2, 3})
	if loaded != nil {
		t.Error("loadCachedIndex should return nil when cache disabled")
	}
}

func TestLoadCachedIndex_noConfig(t *testing.T) {
	loaded := loadCachedIndex(nil, []byte{1, 2, 3})
	if loaded != nil {
		t.Error("loadCachedIndex should return nil when config is nil")
	}
}

func TestIdeasCache_pathResolution(t *testing.T) {
	dir := t.TempDir()
	fakePath := filepath.Join(dir, "ideas.json")

	// Explicit config DataPath always returns the configured path (existence checked by caller)
	cfg := &types.Config{
		Ideas: &types.IdeasConfig{
			DataPath:  fakePath,
			IndexPath: filepath.Join(dir, "ideas.index"),
		},
	}
	if p := resolveIdeasDataPath(cfg); p != fakePath {
		t.Errorf("resolveIdeasDataPath with explicit config should return %q, got %q", fakePath, p)
	}

	// Without config, the function checks ~/.config/aigc-cli/ideas.json.
	// If that file happens to exist (user's real setup), it returns the path.
	// We just verify the function doesn't panic with nil config.
	_ = resolveIdeasDataPath(nil)
}
