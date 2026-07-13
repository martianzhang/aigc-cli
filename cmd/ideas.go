package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

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

// ideasCmd represents the `aigc-cli ideas` command.
var ideasCmd = &cobra.Command{
	Use:          "ideas [keywords]",
	Short:        "Search AI image prompt ideas from local ideas.json",
	SilenceUsage: true,
	Long: `Search AI image generation prompt ideas from a local ideas.json file.

Outputs markdown by default, with each result containing
reference images, full prompt text, and metadata.

Keywords can be passed as arguments or via stdin:

  aigc-cli ideas "cinematic portrait"
  aigc-cli ideas "luxury perfume" --limit 3
  aigc-cli ideas --random              # random ideas without keywords
  aigc-cli ideas --random --limit 1    # single random idea
  echo "cyberpunk city" | aigc-cli ideas
  aigc-cli ideas --json "cat" | jq '.results[].prompt'

Data file: ~/.config/aigc-cli/ideas.json (run "aigc-cli ideas init" to download).`,
	RunE: runIdeas,
}

func runIdeas(cmd *cobra.Command, args []string) error {
	// Resolve keywords
	keywords, err := resolveIdeasKeywords(args)
	if err != nil {
		return err
	}

	// Load ideas.json (from external file)
	entries, rawData, err := loadIdeas()
	if err != nil {
		return err
	}

	// Load or build BM25 index (cache-supported)
	var idx *bm25Index
	if keywords != "" {
		hash := computeHash(rawData)
		idx = loadCachedIndex(shared.Cfg, hash)
		if idx == nil {
			idx = buildBM25Index(entries)
			saveCachedIndex(shared.Cfg, idx, hash)
		}
	}

	// Search
	var results []searchResult
	if ideasFindImage != "" {
		results = searchByImage(entries, ideasFindImage)
		keywords = "图片: " + ideasFindImage
	} else if keywords != "" {
		results = searchIdeas(entries, idx, keywords)
	} else if ideasRandom {
		// --random without keywords: return all entries randomly
		for i := range entries {
			results = append(results, searchResult{entry: entries[i]})
		}
		keywords = "随机灵感"
	} else {
		return fmt.Errorf("keywords or --find-image are required")
	}
	if len(results) == 0 {
		fmt.Println("没有找到匹配的提示词。")
		return nil
	}

	total := len(results)

	// Randomize if --random flag is set (shuffles matched results)
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

	// --preview implies --save: system viewer needs files on disk
	if ideasPreview && !ideasSaveImages {
		ideasSaveImages = true
	}

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

func loadIdeas() (entries []IdeaEntry, rawData []byte, err error) {
	path := resolveIdeasDataPath(shared.Cfg)
	if path == "" {
		return nil, nil, fmt.Errorf("ideas.json not found.\n  Run 'aigc-cli ideas init' to download the prompt dataset,\n  or place ideas.json at ~/.config/aigc-cli/ideas.json")
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, nil, fmt.Errorf("invalid ideas.json: %w", err)
	}
	return entries, data, nil
}

func init() {
	f := ideasCmd.Flags()
	f.IntVarP(&ideasLimit, "limit", "l", ideasDefaultLimit, "Number of results to show (default 8)")
	f.BoolVar(&ideasRandom, "random", false, "Shuffle matched results randomly (default: ranked by relevance)")
	f.BoolVar(&ideasJSON, "json", false, "Output as JSON instead of markdown")
	f.BoolVar(&ideasSaveImages, "save", false, "Download reference images to local directory")
	f.BoolVar(&ideasPreview, "preview", false, "Open saved images with system default viewer (implies --save)")
	f.StringVar(&ideasFindImage, "find-image", "", "Search by image filename (matches image_urls in dataset)")

	rootCmd.AddCommand(ideasCmd)
}
