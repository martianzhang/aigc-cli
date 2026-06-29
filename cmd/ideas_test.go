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

func TestResolveIdeasKeywords_noArgsNoStdin(t *testing.T) {
	// Stdin is a terminal (no pipe) — should return empty
	got, err := resolveIdeasKeywords(nil)
	if err != nil {
		t.Fatalf("resolveIdeasKeywords() returned error: %v", err)
	}
	if got != "" {
		t.Errorf("resolveIdeasKeywords() = %q, want empty", got)
	}
}

// --- localImagePath ---

func TestLocalImagePath_emptyURL(t *testing.T) {
	if got := localImagePath(0, ""); got != "" {
		t.Errorf("localImagePath() = %q, want empty", got)
	}
}

func TestLocalImagePath_withJPG(t *testing.T) {
	got := localImagePath(0, "https://example.com/img.jpg")
	if !strings.HasSuffix(got, "result-1.jpg") {
		t.Errorf("localImagePath() = %q, want suffix result-1.jpg", got)
	}
	if !strings.Contains(got, "ideas") || !strings.Contains(got, "images") {
		t.Errorf("localImagePath() = %q, should contain ideas/images", got)
	}
}

func TestLocalImagePath_withPNG(t *testing.T) {
	got := localImagePath(4, "https://example.com/img.png")
	if !strings.HasSuffix(got, "result-5.png") {
		t.Errorf("localImagePath() = %q, want suffix result-5.png", got)
	}
}

func TestLocalImagePath_noExt(t *testing.T) {
	got := localImagePath(0, "https://example.com/img")
	if !strings.HasSuffix(got, "result-1.jpg") {
		t.Errorf("localImagePath() = %q, want suffix result-1.jpg (fallback)", got)
	}
}

// --- outputIdeasMarkdown ---

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

func TestOutputIdeasMarkdown_multipleResults(t *testing.T) {
	data := ideasData{
		Total: 2,
		Results: []ideasResult{
			{
				Title:       "Test One",
				Description: "Desc one",
				Prompt:      "Prompt one content",
				Model:       "gpt-image-2",
				ImageURL:    "https://example.com/1.jpg",
				Categories:  []ideasCat{{Slug: "portrait", Title: "Portrait"}},
				Source:      ideasSource{AuthorName: "Alice"},
				DetailURL:   "https://example.com/1",
			},
			{
				Title:       "Test Two",
				Description: "Desc two",
				Prompt:      "Prompt two content",
				Model:       "dall-e-3",
				ImageURL:    "https://example.com/2.jpg",
				Categories:  []ideasCat{{Slug: "landscape", Title: "Landscape"}},
				Source:      ideasSource{AuthorName: "Bob"},
				DetailURL:   "https://example.com/2",
			},
		},
	}

	output := captureStdout(func() {
		if err := outputIdeasMarkdown(data, "test keywords", "https://image2studio.com/prompts?q=test"); err != nil {
			t.Errorf("outputIdeasMarkdown() returned error: %v", err)
		}
	})

	tests := []struct {
		name string
		want string
	}{
		{"title", "# Ideas: test keywords"},
		{"search link", "[在线浏览]"},
		{"first result heading", "## Test One"},
		{"second result heading", "## Test Two"},
		{"prompt in code block", "```\nPrompt one content\n```"},
		{"category tag", "`#portrait`"},
		{"first author", "作者: Alice"},
		{"second author", "作者: Bob"},
		{"model", "模型: gpt-image-2"},
		{"detail link", "[详情]"},
		{"separator between results", "---"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(output, tt.want) {
				t.Errorf("output missing %q", tt.want)
			}
		})
	}
}

func TestOutputIdeasMarkdown_singleResultNoSeparator(t *testing.T) {
	data := ideasData{
		Total: 1,
		Results: []ideasResult{
			{Title: "Only One", Prompt: "Single prompt"},
		},
	}

	output := captureStdout(func() {
		if err := outputIdeasMarkdown(data, "test", "https://example.com/search"); err != nil {
			t.Errorf("outputIdeasMarkdown() returned error: %v", err)
		}
	})

	if !strings.Contains(output, "## Only One") {
		t.Errorf("output missing result title")
	}
	if strings.Contains(output, "---") {
		t.Errorf("single result should not have separator")
	}
}

func TestOutputIdeasMarkdown_emptyTitle(t *testing.T) {
	data := ideasData{
		Total: 1,
		Results: []ideasResult{
			{Title: "", Prompt: "just a prompt"},
		},
	}

	output := captureStdout(func() {
		if err := outputIdeasMarkdown(data, "test", "https://example.com/search"); err != nil {
			t.Errorf("outputIdeasMarkdown() returned error: %v", err)
		}
	})

	if !strings.Contains(output, "## Result 1") {
		t.Errorf("empty title should fallback to 'Result 1', got:\n%s", output)
	}
}

func TestOutputIdeasMarkdown_saveModeLocalPath(t *testing.T) {
	// Enable save mode
	ideasSaveImages = true
	defer func() { ideasSaveImages = false }()

	data := ideasData{
		Total: 1,
		Results: []ideasResult{
			{Title: "Local", Prompt: "test", ImageURL: "https://example.com/img.jpg"},
		},
	}

	output := captureStdout(func() {
		if err := outputIdeasMarkdown(data, "test", "https://example.com/search"); err != nil {
			t.Errorf("outputIdeasMarkdown() returned error: %v", err)
		}
	})

	if !strings.Contains(output, "result-1.jpg") {
		t.Errorf("save mode should use local path, got:\n%s", output)
	}
	if !strings.Contains(output, "ideas") || !strings.Contains(output, "images") {
		t.Errorf("save mode should reference ideas/images dir, got:\n%s", output)
	}
}

// --- outputIdeasJSON ---

func TestOutputIdeasJSON_single(t *testing.T) {
	data := ideasData{
		Total: 1,
		Results: []ideasResult{
			{Title: "JSON Test", Prompt: "Test prompt for JSON output"},
		},
	}

	output := captureStdout(func() {
		if err := outputIdeasJSON(data); err != nil {
			t.Errorf("outputIdeasJSON() returned error: %v", err)
		}
	})

	var parsed struct {
		Total   int           `json:"total"`
		Results []ideasResult `json:"results"`
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
		t.Errorf("title = %q, want %q", parsed.Results[0].Title, "JSON Test")
	}
}

func TestOutputIdeasJSON_multiple(t *testing.T) {
	data := ideasData{
		Total: 10,
		Results: []ideasResult{
			{Title: "A", Prompt: "test a"},
			{Title: "B", Prompt: "test b"},
		},
	}

	output := captureStdout(func() {
		if err := outputIdeasJSON(data); err != nil {
			t.Errorf("outputIdeasJSON() returned error: %v", err)
		}
	})

	var parsed struct {
		Total   int           `json:"total"`
		Results []ideasResult `json:"results"`
	}
	json.Unmarshal([]byte(output), &parsed)
	if parsed.Total != 10 {
		t.Errorf("total = %d, want 10", parsed.Total)
	}
	if len(parsed.Results) != 2 {
		t.Errorf("results = %d, want 2", len(parsed.Results))
	}
}

// --- saveResultImages ---

func TestSaveResultImages_nilOrEmpty(t *testing.T) {
	if err := saveResultImages(nil); err != nil {
		t.Errorf("saveResultImages(nil) = %v", err)
	}
	if err := saveResultImages([]ideasResult{}); err != nil {
		t.Errorf("saveResultImages(empty) = %v", err)
	}
}

func TestSaveResultImages_createsDir(t *testing.T) {
	tmpDir := t.TempDir()
	oldOutput := shared.OutputDir
	shared.OutputDir = tmpDir
	defer func() { shared.OutputDir = oldOutput }()

	// Empty image URL — should skip gracefully
	err := saveResultImages([]ideasResult{
		{Title: "No Image", ImageURL: ""},
	})
	if err != nil {
		t.Errorf("saveResultImages() = %v", err)
	}
	// Verify dir was created
	if _, err := os.Stat(tmpDir + "/ideas/images"); os.IsNotExist(err) {
		t.Errorf("images directory was not created")
	}
}

// --- default constants ---

func TestDefaultConstants(t *testing.T) {
	if ideasDefaultLimit != 8 {
		t.Errorf("ideasDefaultLimit = %d, want 8", ideasDefaultLimit)
	}
	if ideasDefaultPageSize != 8 {
		t.Errorf("ideasDefaultPageSize = %d, want 8", ideasDefaultPageSize)
	}
	if ideasAPIBase != "https://api.image2studio.com/public/prompts/search" {
		t.Errorf("ideasAPIBase changed: %s", ideasAPIBase)
	}
	if ideasWebBase != "https://image2studio.com/prompts" {
		t.Errorf("ideasWebBase changed: %s", ideasWebBase)
	}
}
