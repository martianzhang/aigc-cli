package cmd

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/ideas"
)

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
	Short:        "Search AI image prompt ideas from local index",
	SilenceUsage: true,
	Long: `Search AI image generation prompt ideas from a locally indexed database.

Outputs markdown by default, with each result containing
reference images, full prompt text, and metadata.

Keywords can be passed as arguments or via stdin:

  aigc-cli ideas "cinematic portrait"
  aigc-cli ideas "luxury perfume" --limit 3
  aigc-cli ideas --random              # random ideas without keywords
  aigc-cli ideas --random --limit 1    # single random idea
  echo "cyberpunk city" | aigc-cli ideas
  aigc-cli ideas --json "cat" | jq '.results[].prompt'

Data: ~/.config/aigc-cli/ideas/ (run "aigc-cli ideas init" to download and index).`,
	RunE: runIdeas,
}

func runIdeas(cmd *cobra.Command, args []string) error {
	keywords, err := resolveIdeasKeywords(args)
	if err != nil {
		return err
	}

	db, err := ideas.OpenDB(resolveIdeasDBPath(shared.Cfg))
	if err != nil {
		return fmt.Errorf("ideas database not found: %w\n  Run 'aigc-cli ideas init' to download and build it", err)
	}
	defer db.Close()

	var results []ideas.SearchResult
	var total int

	if ideasFindImage != "" {
		entries, err := ideas.SearchEntriesByImage(db, ideasFindImage)
		if err != nil {
			return fmt.Errorf("failed to search by image: %w", err)
		}
		keywords = "图片: " + ideasFindImage
		for _, e := range entries {
			results = append(results, ideas.SearchResult{Entry: e, Score: 1})
		}
		total = len(results)
	} else if keywords != "" {
		results, total, err = ideas.SearchDBResults(db, keywords, ideasLimit)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}
	} else {
		// No keywords and no --find-image: random.
		ideasRandom = true
		limit := ideasLimit
		if limit <= 0 {
			limit = ideasDefaultLimit
		}
		entries, err := ideas.LoadRandomEntries(db, limit)
		if err != nil {
			return fmt.Errorf("failed to load random entries: %w", err)
		}
		keywords = "随机灵感"
		for _, e := range entries {
			results = append(results, ideas.SearchResult{Entry: e})
		}
		total = len(results)
	}

	if len(results) == 0 {
		fmt.Println("没有找到匹配的提示词。")
		return nil
	}

	if ideasRandom {
		rand.Shuffle(len(results), func(i, j int) {
			results[i], results[j] = results[j], results[i]
		})
	}

	limit := ideasLimit
	if limit > total {
		limit = total
	}
	results = results[:limit]

	if ideasPreview && !ideasSaveImages {
		ideasSaveImages = true
	}

	if ideasSaveImages {
		var entries []ideas.IdeaEntry
		for _, r := range results {
			entries = append(entries, r.Entry)
		}
		saved, _ := saveIdeaImages(entries)
		if ideasJSON {
			return outputJSON(results, total)
		}
		return outputMarkdown(results, keywords, total, saved)
	}

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

func init() {
	f := ideasCmd.Flags()
	f.IntVarP(&ideasLimit, "limit", "l", ideasDefaultLimit, "Number of results to show")
	f.BoolVar(&ideasRandom, "random", false, "Shuffle matched results randomly (default: ranked by relevance)")
	f.BoolVar(&ideasJSON, "json", false, "Output as JSON instead of markdown")
	f.BoolVar(&ideasSaveImages, "save", false, "Download reference images to local directory")
	f.BoolVar(&ideasPreview, "preview", false, "Open saved images with system default viewer (implies --save)")
	f.StringVar(&ideasFindImage, "find-image", "", "Search by image filename (matches image_urls in dataset)")

	rootCmd.AddCommand(ideasCmd)
}
