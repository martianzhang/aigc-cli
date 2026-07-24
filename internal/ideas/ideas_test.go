package ideas

import (
	"math"
	"strings"
	"testing"
)

func TestNgramSet(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		n        int
		expected map[string]int
	}{
		{
			name: "hello world n=3",
			s:    "hello world",
			n:    3,
			expected: map[string]int{
				"hel": 1, "ell": 1, "llo": 1, "lo ": 1,
				"o w": 1, " wo": 1, "wor": 1, "orl": 1, "rld": 1,
			},
		},
		{
			name:     "string shorter than n",
			s:        "hi",
			n:        3,
			expected: map[string]int{},
		},
		{
			name:     "empty string",
			s:        "",
			n:        3,
			expected: map[string]int{},
		},
		{
			name: "repeated grams",
			s:    "ababab",
			n:    2,
			expected: map[string]int{
				"ab": 3, "ba": 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ngramSet(tt.s, tt.n)
			if len(got) != len(tt.expected) {
				t.Fatalf("ngramSet(%q, %d) = %v, want %v", tt.s, tt.n, got, tt.expected)
			}
			for k, v := range tt.expected {
				if got[k] != v {
					t.Fatalf("ngramSet(%q, %d)[%q] = %d, want %d", tt.s, tt.n, k, got[k], v)
				}
			}
		})
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        map[string]int
		b        map[string]int
		expected float64
		epsilon  float64
	}{
		{
			name:     "identical maps",
			a:        map[string]int{"a": 1, "b": 2},
			b:        map[string]int{"a": 1, "b": 2},
			expected: 1.0,
			epsilon:  1e-9,
		},
		{
			name:     "completely different",
			a:        map[string]int{"a": 1},
			b:        map[string]int{"b": 1},
			expected: 0.0,
			epsilon:  0.0,
		},
		{
			name:     "partial overlap",
			a:        map[string]int{"a": 1, "b": 1},
			b:        map[string]int{"a": 1, "c": 1},
			expected: 0.5,
			epsilon:  1e-9,
		},
		{
			name:     "empty a",
			a:        map[string]int{},
			b:        map[string]int{"a": 1},
			expected: 0.0,
			epsilon:  0.0,
		},
		{
			name:     "empty b",
			a:        map[string]int{"a": 1},
			b:        map[string]int{},
			expected: 0.0,
			epsilon:  0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineSimilarity(tt.a, tt.b)
			if math.Abs(got-tt.expected) > tt.epsilon {
				t.Fatalf("cosineSimilarity(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}

func TestContainsWord(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		term     string
		expected bool
	}{
		{name: "exact word match", text: "hello world", term: "world", expected: true},
		{name: "substring no boundary", text: "hello world", term: "wor", expected: false},
		{name: "prefix no boundary", text: "hello world", term: "hell", expected: false},
		{name: "suffix no boundary", text: "hello world", term: "orld", expected: false},
		{name: "CJK whole word", text: "你好世界", term: "你好", expected: true},
		{name: "CJK single char", text: "你好世界", term: "你", expected: true},
		{name: "CJK substring", text: "你好世界", term: "好世", expected: true},
		{name: "single ASCII char no boundary", text: "hello", term: "h", expected: false},
		{name: "single ASCII char at end", text: "hello a", term: "a", expected: true},
		{name: "empty term", text: "hello", term: "", expected: false},
		{name: "whitespace term", text: "hello", term: " ", expected: false},
		{name: "case insensitive", text: "Hello World", term: "world", expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsWord(tt.text, tt.term)
			if got != tt.expected {
				t.Fatalf("containsWord(%q, %q) = %v, want %v", tt.text, tt.term, got, tt.expected)
			}
		})
	}
}

func TestSplitTokens(t *testing.T) {
	tests := []struct {
		name     string
		buf      []rune
		expected []string
	}{
		{name: "single CJK char", buf: []rune{'你'}, expected: []string{"你"}},
		{name: "single ASCII char", buf: []rune{'a'}, expected: []string{"a"}},
		{name: "two CJK chars", buf: []rune{'你', '好'}, expected: []string{"你好"}},
		{name: "four CJK chars 2-grams", buf: []rune{'你', '好', '世', '界'}, expected: []string{"你好", "好世", "世界"}},
		{name: "mixed CJK+ASCII", buf: []rune{'你', 'a'}, expected: []string{"你a"}},
		{name: "ASCII word", buf: []rune{'h', 'e', 'l', 'l', 'o'}, expected: []string{"hello"}},
		{name: "empty buffer", buf: []rune{}, expected: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitTokens(tt.buf)
			if len(got) != len(tt.expected) {
				t.Fatalf("splitTokens(%v) = %v, want %v", tt.buf, got, tt.expected)
			}
			for i := range tt.expected {
				if got[i] != tt.expected[i] {
					t.Fatalf("splitTokens(%v)[%d] = %q, want %q", tt.buf, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{name: "ASCII words", text: "hello world", expected: []string{"hello", "world"}},
		{name: "CJK 4 chars", text: "你好世界", expected: []string{"你好", "好世", "世界"}},
		{name: "CJK 2 chars", text: "测试", expected: []string{"测试"}},
		{name: "mixed ASCII and CJK", text: "a 测试 b", expected: []string{"a", "测试", "b"}},
		{name: "punctuation splits", text: "hello,world", expected: []string{"hello", "world"}},
		{name: "multiple spaces", text: "hello   world", expected: []string{"hello", "world"}},
		{name: "uppercase to lowercase", text: "Hello World", expected: []string{"hello", "world"}},
		{name: "digits", text: "test123", expected: []string{"test123"}},
		{name: "empty string", text: "", expected: nil},
		{name: "only punctuation", text: ",.;!", expected: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenize(tt.text)
			if len(got) != len(tt.expected) {
				t.Fatalf("tokenize(%q) = %v, want %v", tt.text, got, tt.expected)
			}
			for i := range tt.expected {
				if got[i] != tt.expected[i] {
					t.Fatalf("tokenize(%q)[%d] = %q, want %q", tt.text, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestSearchableText(t *testing.T) {
	e := IdeaEntry{
		Title:    "Title",
		TitleZh:  "标题",
		Prompt:   "Prompt",
		PromptZh: "提示词",
	}
	got := searchableText(e)
	want := "Title 标题 Prompt 提示词"
	if got != want {
		t.Fatalf("searchableText() = %q, want %q", got, want)
	}
}

func TestAndFilter(t *testing.T) {
	tests := []struct {
		name     string
		docSet   map[string]int
		terms    []string
		expected bool
	}{
		{name: "all present", docSet: map[string]int{"a": 1, "b": 2}, terms: []string{"a", "b"}, expected: true},
		{name: "one missing", docSet: map[string]int{"a": 1}, terms: []string{"a", "b"}, expected: false},
		{name: "empty terms", docSet: map[string]int{"a": 1}, terms: []string{}, expected: true},
		{name: "empty docSet missing", docSet: map[string]int{}, terms: []string{"a"}, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := andFilter(tt.docSet, tt.terms)
			if got != tt.expected {
				t.Fatalf("andFilter(%v, %v) = %v, want %v", tt.docSet, tt.terms, got, tt.expected)
			}
		})
	}
}

func TestBuildBM25Index(t *testing.T) {
	entries := []IdeaEntry{
		{Title: "cat photo", Prompt: "a cute cat sitting on a mat", Lang: "en"},
		{Title: "dog photo", Prompt: "a happy dog running in park", Lang: "en"},
	}

	idx := BuildBM25Index(entries)
	if idx == nil {
		t.Fatal("BuildBM25Index returned nil")
	}
	if idx.docCount != 2 {
		t.Fatalf("docCount = %d, want 2", idx.docCount)
	}
	if idx.avgDocLen <= 0 {
		t.Fatalf("avgDocLen = %f, want > 0", idx.avgDocLen)
	}
	if len(idx.idf) == 0 {
		t.Fatal("idf map is empty")
	}
	if len(idx.docTokens) != 2 {
		t.Fatalf("len(docTokens) = %d, want 2", len(idx.docTokens))
	}
	if len(idx.docSet) != 2 {
		t.Fatalf("len(docSet) = %d, want 2", len(idx.docSet))
	}

	// Verify idf values are positive
	for term, idf := range idx.idf {
		if idf <= 0 {
			t.Fatalf("idf[%q] = %f, want > 0", term, idf)
		}
	}

	// Verify docTokens are populated
	for i, tokens := range idx.docTokens {
		if len(tokens) == 0 {
			t.Fatalf("docTokens[%d] is empty", i)
		}
	}
}

func TestBuildBM25IndexEmpty(t *testing.T) {
	idx := BuildBM25Index([]IdeaEntry{})
	if idx == nil {
		t.Fatal("BuildBM25Index([]) returned nil")
	}
	if idx.docCount != 0 {
		t.Fatalf("docCount = %d, want 0", idx.docCount)
	}
	if idx.avgDocLen != 0 {
		t.Fatalf("avgDocLen = %f, want 0", idx.avgDocLen)
	}
	if len(idx.idf) != 0 {
		t.Fatalf("len(idf) = %d, want 0", len(idx.idf))
	}
}

func TestSearchByImage(t *testing.T) {
	entries := []IdeaEntry{
		{
			Title:     "test",
			Prompt:    "test prompt",
			Lang:      "en",
			ImageURLs: []string{"https://example.com/img.jpg"},
			SourceURL: "https://example.com",
			Author:    "Author",
			License:   "MIT",
		},
		{
			Title:     "other",
			Prompt:    "other prompt",
			Lang:      "en",
			ImageURLs: []string{"https://example.com/other.png"},
			SourceURL: "https://example.com/other",
			Author:    "Author2",
			License:   "CC0",
		},
	}

	t.Run("exact match", func(t *testing.T) {
		got := SearchByImage(entries, "img.jpg")
		if len(got) != 1 {
			t.Fatalf("SearchByImage(..., img.jpg) returned %d results, want 1", len(got))
		}
		if got[0].Entry.Title != "test" {
			t.Fatalf("result title = %q, want test", got[0].Entry.Title)
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		got := SearchByImage(entries, "IMG.JPG")
		if len(got) != 1 {
			t.Fatalf("SearchByImage(..., IMG.JPG) returned %d results, want 1", len(got))
		}
	})

	t.Run("no match", func(t *testing.T) {
		got := SearchByImage(entries, "notfound.gif")
		if len(got) != 0 {
			t.Fatalf("SearchByImage(..., notfound.gif) returned %d results, want 0", len(got))
		}
	})

	t.Run("deduplication by URL", func(t *testing.T) {
		// Two entries with same image URL should deduplicate
		dupEntries := []IdeaEntry{
			{Title: "a", Prompt: "pa", Lang: "en", ImageURLs: []string{"https://x.com/same.jpg"}},
			{Title: "b", Prompt: "pb", Lang: "en", ImageURLs: []string{"https://x.com/same.jpg"}},
		}
		got := SearchByImage(dupEntries, "same.jpg")
		if len(got) != 1 {
			t.Fatalf("SearchByImage dedup returned %d results, want 1", len(got))
		}
	})
}

func TestFormatResultsMarkdown(t *testing.T) {
	entry := IdeaEntry{
		Title:     "Test Title",
		Prompt:    "Test Prompt",
		Lang:      "en",
		ImageURLs: []string{"https://example.com/img.jpg"},
		SourceURL: "https://example.com",
		Author:    "TestAuthor",
		License:   "MIT",
	}

	t.Run("basic output", func(t *testing.T) {
		results := []SearchResult{{Entry: entry, Score: 100}}
		got := FormatResultsMarkdown(results, "test", 1)
		if !strings.Contains(got, "Test Title") {
			t.Fatal("missing title")
		}
		if !strings.Contains(got, "Test Prompt") {
			t.Fatal("missing prompt")
		}
		if !strings.Contains(got, "Author: TestAuthor") {
			t.Fatal("missing author")
		}
		if !strings.Contains(got, "[Source](https://example.com)") {
			t.Fatal("missing source URL")
		}
		if !strings.Contains(got, "MIT") {
			t.Fatal("missing license")
		}
		if !strings.Contains(got, "![ref](https://example.com/img.jpg)") {
			t.Fatal("missing image")
		}
	})

	t.Run("zh lang uses PromptZh", func(t *testing.T) {
		zhEntry := entry
		zhEntry.Lang = "zh"
		zhEntry.PromptZh = "中文提示词"
		results := []SearchResult{{Entry: zhEntry, Score: 100}}
		got := FormatResultsMarkdown(results, "test", 1)
		if !strings.Contains(got, "中文提示词") {
			t.Fatal("missing zh prompt")
		}
		if strings.Contains(got, "Test Prompt") {
			t.Fatal("should not contain en prompt when zh is available")
		}
	})

	t.Run("empty results", func(t *testing.T) {
		got := FormatResultsMarkdown([]SearchResult{}, "test", 0)
		if !strings.Contains(got, "Found 0 result(s)") {
			t.Fatalf("unexpected output: %q", got)
		}
	})

	t.Run("total greater than results", func(t *testing.T) {
		results := []SearchResult{{Entry: entry, Score: 100}}
		got := FormatResultsMarkdown(results, "test", 5)
		if !strings.Contains(got, "(showing 1/5)") {
			t.Fatalf("missing showing clause: %q", got)
		}
	})

	t.Run("separator between multiple results", func(t *testing.T) {
		results := []SearchResult{
			{Entry: entry, Score: 100},
			{Entry: entry, Score: 90},
		}
		got := FormatResultsMarkdown(results, "test", 2)
		if !strings.Contains(got, "---") {
			t.Fatal("missing separator between results")
		}
	})

	t.Run("empty title fallback", func(t *testing.T) {
		noTitle := entry
		noTitle.Title = ""
		results := []SearchResult{{Entry: noTitle, Score: 100}}
		got := FormatResultsMarkdown(results, "test", 1)
		if !strings.Contains(got, "## Result 1") {
			t.Fatalf("missing fallback title: %q", got)
		}
	})
}
