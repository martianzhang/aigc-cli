package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	_ "embed"

	"github.com/martianzhang/apimart-cli/internal/service"
)

//go:embed ideas.json
var ideasData []byte

// --- data structures ---

// IdeaEntry represents a single prompt entry in ideas.json.
type IdeaEntry struct {
	Title     string   `json:"title,omitempty"`
	TitleZh   string   `json:"title_zh,omitempty"`
	Prompt    string   `json:"prompt"`
	PromptZh  string   `json:"prompt_zh,omitempty"`
	ImageURLs []string `json:"image_urls,omitempty"`
	SourceURL string   `json:"source_url,omitempty"`
	Author    string   `json:"author,omitempty"`
	License   string   `json:"license,omitempty"`
	Lang      string   `json:"lang"`
}

// searchResult pairs an entry with its relevance score.
type searchResult struct {
	entry IdeaEntry
	score int
}

// ideas flag variables
var (
	ideasLimit      int
	ideasRandom     bool
	ideasJSON       bool
	ideasSaveImages bool
	ideasPreview    bool
	ideasFindImage  string
)

const ideasDefaultLimit = 8

// ideasCmd represents the `apimart-cli ideas` command.
var ideasCmd = &cobra.Command{
	Use:          "ideas [keywords]",
	Short:        "Search AI image prompt ideas from local ideas.json",
	SilenceUsage: true,
	Long: `Search AI image generation prompt ideas from a local ideas.json file.

Outputs markdown by default, with each result containing
reference images, full prompt text, and metadata.

Keywords can be passed as arguments or via stdin:

  apimart-cli ideas "cinematic portrait"
  apimart-cli ideas "luxury perfume" --limit 3
  echo "cyberpunk city" | apimart-cli ideas
  apimart-cli ideas --json "cat" | jq '.results[].prompt'

Data file: ideas.json in the working directory (generate with "make ideas-data").`,
	RunE: runIdeas,
}

func runIdeas(cmd *cobra.Command, args []string) error {
	// Resolve keywords
	keywords, err := resolveIdeasKeywords(args)
	if err != nil {
		return err
	}
	if keywords == "" && ideasFindImage == "" {
		return fmt.Errorf("keywords or --find-image are required")
	}

	// Load ideas.json
	entries, err := loadIdeas()
	if err != nil {
		return fmt.Errorf("failed to load ideas.json: %w\n  Generate it with: make ideas-data", err)
	}

	// Search
	var results []searchResult
	if ideasFindImage != "" {
		results = searchByImage(entries, ideasFindImage)
		keywords = "图片: " + ideasFindImage
	} else {
		results = searchIdeas(entries, keywords)
	}
	if len(results) == 0 {
		fmt.Println("没有找到匹配的提示词。")
		return nil
	}

	total := len(results)

	// Randomize if requested (before slicing)
	if ideasRandom {
		rand.Shuffle(len(results), func(i, j int) {
			results[i], results[j] = results[j], results[i]
		})
	}

	// Apply limit
	limit := ideasLimit
	if limit > total {
		limit = total
	}
	results = results[:limit]

	// Save images if requested
	if ideasSaveImages {
		var entries []IdeaEntry
		for _, r := range results {
			entries = append(entries, r.entry)
		}
		saved, _ := saveIdeaImages(entries)
		if ideasJSON {
			return outputJSON(results, total)
		}
		return outputMarkdown(results, keywords, total, saved)
	}

	// Output
	if ideasJSON {
		return outputJSON(results, total)
	}
	return outputMarkdown(results, keywords, total, nil)
}

// --- keyword resolution ---

func resolveIdeasKeywords(args []string) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", nil
	}
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return "", nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("failed to read stdin: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// --- data loading ---

func loadIdeas() ([]IdeaEntry, error) {
	var entries []IdeaEntry
	if err := json.Unmarshal(ideasData, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// --- search ---

func searchIdeas(entries []IdeaEntry, query string) []searchResult {
	keywords := strings.Fields(strings.ToLower(query))
	if len(keywords) == 0 {
		return nil
	}
	var results []searchResult
	for _, e := range entries {
		if score := scoreEntry(e, keywords); score > 0 {
			results = append(results, searchResult{entry: e, score: score})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })
	return results
}

// searchByImage finds entries whose image_urls contain the given filename.
func searchByImage(entries []IdeaEntry, filename string) []searchResult {
	fn := strings.ToLower(filename)
	seen := make(map[string]bool)
	var results []searchResult
	for _, e := range entries {
		for _, url := range e.ImageURLs {
			if strings.Contains(strings.ToLower(url), fn) {
				// Dedup by image URL; fallback to source_url or title+prompt
				key := url
				if key == "" {
					key = e.SourceURL
				}
				if key == "" {
					key = e.Title + "|" + e.Prompt
				}
				if !seen[key] {
					seen[key] = true
					results = append(results, searchResult{entry: e, score: 1})
				}
				break
			}
		}
	}
	return results
}

func scoreEntry(e IdeaEntry, keywords []string) int {
	haystack := strings.ToLower(e.Title + " " + e.TitleZh + " " + e.Prompt + " " + e.PromptZh)
	score := 0
	for _, kw := range keywords {
		if strings.Contains(haystack, kw) {
			score++
			if strings.Contains(strings.ToLower(e.Title), kw) ||
				strings.Contains(strings.ToLower(e.TitleZh), kw) {
				score += 2
			}
		}
	}
	return score
}

// --- markdown output ---

func outputMarkdown(results []searchResult, keywords string, total int, savedFiles []string) error {
	now := time.Now().Format("2006-01-02")
	header := fmt.Sprintf("# Ideas: %s\n> 找到 %d 个结果 · %s", keywords, total, now)
	if total > len(results) {
		header += fmt.Sprintf(" · 显示 %d/%d", len(results), total)
	}
	fmt.Println(header)
	fmt.Println()

	for i, r := range results {
		if i > 0 {
			fmt.Println("---")
			fmt.Println()
		}
		e := r.entry
		title := e.Title
		if title == "" {
			title = fmt.Sprintf("Result %d", i+1)
		}
		fmt.Printf("## %s\n\n", title)

		// Prompt (prefer zh for zh entries)
		prompt := e.Prompt
		if e.Lang == "zh" && e.PromptZh != "" {
			prompt = e.PromptZh
		}
		fmt.Printf("**提示词：**\n```\n%s\n```\n\n", prompt)

		// Images
		if len(e.ImageURLs) > 0 {
			if ideasSaveImages {
				for j, url := range e.ImageURLs {
					fmt.Printf("![参考图 %d](%s)\n\n", j+1, localImagePath(url))
				}
			} else {
				for j, url := range e.ImageURLs {
					fmt.Printf("![参考图 %d](%s)\n\n", j+1, url)
				}
			}
		}

		// Inline preview: show saved images right after their entry
		if ideasPreview && len(savedFiles) > 0 {
			for range e.ImageURLs {
				if len(savedFiles) == 0 {
					break
				}
				f := savedFiles[0]
				savedFiles = savedFiles[1:]
				if e := service.PreviewFile(f); e != nil {
					fmt.Fprintf(os.Stderr, "Warning: preview failed: %v\n", e)
				}
			}
		}

		// Metadata
		var meta []string
		if e.Author != "" {
			meta = append(meta, "作者: "+e.Author)
		}
		if e.SourceURL != "" {
			meta = append(meta, fmt.Sprintf("[来源](%s)", e.SourceURL))
		}
		if e.License != "" {
			meta = append(meta, e.License)
		}
		if len(meta) > 0 {
			fmt.Printf("%s\n\n", strings.Join(meta, " · "))
		}
	}
	return nil
}

// --- json output ---

func outputJSON(results []searchResult, total int) error {
	out := struct {
		Total   int         `json:"total"`
		Results []IdeaEntry `json:"results"`
	}{Total: total}
	for _, r := range results {
		out.Results = append(out.Results, r.entry)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// --- image saving ---

func saveIdeaImages(entries []IdeaEntry) ([]string, error) {
	var saved []string
	if err := os.MkdirAll(shared.OutputDir, 0755); err != nil {
		return saved, fmt.Errorf("cannot create output directory: %w", err)
	}
	for _, e := range entries {
		for _, imgURL := range e.ImageURLs {
			if imgURL == "" {
				continue
			}
			name := filepath.Base(imgURL)
			path := filepath.Join(shared.OutputDir, name)
			// Skip if already exists
			if _, err := os.Stat(path); err == nil {
				saved = append(saved, path)
				continue
			}
			resp, err := http.DefaultClient.Get(imgURL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download %s: %v\n", imgURL, err)
				continue
			}
			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to read %s: %v\n", imgURL, err)
				continue
			}
			if err := os.WriteFile(path, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", name, err)
				continue
			}
			saved = append(saved, path)
		}
	}
	return saved, nil
}

func localImagePath(remoteURL string) string {
	if remoteURL == "" {
		return ""
	}
	return filepath.Join(shared.OutputDir, filepath.Base(remoteURL))
}

func init() {
	f := ideasCmd.Flags()
	f.IntVarP(&ideasLimit, "limit", "l", ideasDefaultLimit, "Number of results to show (default 8)")
	f.BoolVar(&ideasRandom, "random", true, "Shuffle results randomly (from full result set, default true)")
	f.BoolVar(&ideasJSON, "json", false, "Output as JSON instead of markdown")
	f.BoolVar(&ideasSaveImages, "save", false, "Download reference images to local directory")
	f.BoolVar(&ideasPreview, "preview", false, "Open saved images with system default viewer (implies --save)")
	f.StringVar(&ideasFindImage, "find-image", "", "Search by image filename (matches image_urls in dataset)")

	rootCmd.AddCommand(ideasCmd)
}
