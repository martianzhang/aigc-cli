// Package ideas implements the `aigc-cli ideas` command.
package ideas

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/ideas"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// Deps carries the runtime configuration the ideas command needs.
type Deps struct {
	Cfg       *types.Config
	OutputDir string
}

const (
	defaultLimit = 8
	dataURL      = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1/ideas.json"
)

type cmdFlags struct {
	limit     int
	random    bool
	jsonOut   bool
	save      bool
	preview   bool
	findImage string
}

// NewCommand builds the `ideas` command tree. deps is resolved at run time.
func NewCommand(deps func() Deps) *cobra.Command {
	var f cmdFlags

	cmd := &cobra.Command{
		Use:          "ideas [keywords]",
		Aliases:      []string{"idea"},
		Short:        "Search AI image prompt ideas (also: idea)",
		SilenceUsage: true,
		Long: `Search AI image generation prompt ideas from a local ideas.json file.

Outputs markdown by default, with each result containing
reference images, full prompt text, and metadata.

Keywords can be passed as arguments or via stdin.

Data file: ~/.config/aigc-cli/ideas.json (run "aigc-cli ideas init" to download).`,
		Example: `  aigc-cli ideas "cinematic portrait"
  aigc-cli ideas "luxury perfume" --limit 3
  aigc-cli ideas --random --limit 1              # single random idea
  echo "cyberpunk city" | aigc-cli ideas
  aigc-cli ideas --json "cat" | jq '.results[].prompt'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(deps(), args, &f)
		},
	}

	fl := cmd.Flags()
	fl.IntVarP(&f.limit, "limit", "l", defaultLimit, "Number of results to show (default 8)")
	fl.BoolVar(&f.random, "random", false, "Shuffle matched results randomly (default: ranked by relevance)")
	fl.BoolVar(&f.jsonOut, "json", false, "Output as JSON instead of markdown")
	fl.BoolVar(&f.save, "save", false, "Download reference images to local directory")
	fl.BoolVar(&f.preview, "preview", false, "Open saved images with system default viewer (implies --save)")
	fl.StringVar(&f.findImage, "find-image", "", "Search by image filename (matches image_urls in dataset)")

	cmd.AddCommand(newInitCommand(deps))
	return cmd
}

func run(d Deps, args []string, f *cmdFlags) error {
	keywords, err := resolveKeywords(args)
	if err != nil {
		return err
	}

	entries, err := ideas.LoadIdeas(resolveDataPath(d.Cfg))
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("ideas.json is empty. Run `aigc-cli ideas init` to download.")
		return nil
	}

	var idx *ideas.BM25Index
	if keywords != "" {
		idx = ideas.BuildBM25Index(entries)
	}

	var results []ideas.SearchResult
	if f.findImage != "" {
		results = ideas.SearchByImage(entries, f.findImage)
		keywords = "图片: " + f.findImage
	} else if keywords != "" {
		results = ideas.SearchIdeas(entries, idx, keywords)
	} else {
		f.random = true
		f.limit = 1
		for i := range entries {
			results = append(results, ideas.SearchResult{Entry: entries[i]})
		}
		keywords = "随机灵感"
	}
	if len(results) == 0 {
		fmt.Println("没有找到匹配的提示词。")
		return nil
	}

	total := len(results)

	if f.random {
		rand.Shuffle(len(results), func(i, j int) {
			results[i], results[j] = results[j], results[i]
		})
	}

	limit := f.limit
	if limit > total {
		limit = total
	}
	results = results[:limit]

	if f.preview && !f.save {
		f.save = true
	}

	if f.save {
		var imgEntries []ideas.IdeaEntry
		for _, r := range results {
			imgEntries = append(imgEntries, r.Entry)
		}
		saved, _ := saveIdeaImages(imgEntries, d.OutputDir)
		if f.jsonOut {
			return outputJSON(results, total)
		}
		return outputMarkdown(results, keywords, total, saved, f.preview)
	}

	if f.jsonOut {
		return outputJSON(results, total)
	}
	return outputMarkdown(results, keywords, total, nil, f.preview)
}

func newInitCommand(deps func() Deps) *cobra.Command {
	return &cobra.Command{
		Use:          "init",
		Short:        "Download ideas data",
		SilenceUsage: true,
		Long: `Download the AI image prompt ideas dataset.

The data is saved to ~/.config/aigc-cli/ideas/ideas.json (or the configured ideas.data_path).

Proxy settings from config.yaml, env vars (HTTP_PROXY), or --http-proxy flag
are automatically respected.`,
		Example: `  aigc-cli ideas init`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(deps())
		},
	}
}

// --- keyword resolution ---

func resolveKeywords(args []string) (string, error) {
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

// --- data path helpers ---

func ideasDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "aigc-cli"), nil
}

func resolveDataPath(cfg *types.Config) string {
	if cfg != nil && cfg.Ideas != nil && cfg.Ideas.DataPath != "" {
		return cfg.Ideas.DataPath
	}
	dir, err := ideasDir()
	if err != nil {
		return ""
	}
	p := filepath.Join(dir, "ideas", "ideas.json")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

func dataSavePath(cfg *types.Config) string {
	if cfg != nil && cfg.Ideas != nil && cfg.Ideas.DataPath != "" {
		return cfg.Ideas.DataPath
	}
	dir, err := ideasDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "ideas", "ideas.json")
}

// --- image + output helpers ---

func saveIdeaImages(entries []ideas.IdeaEntry, outputDir string) ([]string, error) {
	var saved []string
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return saved, fmt.Errorf("cannot create output directory: %w", err)
	}
	for _, e := range entries {
		for _, imgURL := range e.ImageURLs {
			if imgURL == "" {
				continue
			}
			path := filepath.Join(outputDir, filepath.Base(imgURL))
			if _, err := os.Stat(path); err == nil {
				saved = append(saved, path)
				continue
			}
			if err := service.SaveResource(imgURL, path); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download %s: %v\n", imgURL, err)
				continue
			}
			saved = append(saved, path)
		}
	}
	return saved, nil
}

func localImagePath(remoteURL, outputDir string) string {
	if remoteURL == "" {
		return ""
	}
	return filepath.Join(outputDir, filepath.Base(remoteURL))
}

func outputMarkdown(results []ideas.SearchResult, keywords string, total int, savedFiles []string, preview bool) error {
	md := ideas.FormatResultsMarkdown(results, keywords, total)
	fmt.Println(md)

	for _, r := range results {
		if preview && len(savedFiles) > 0 {
			for range r.Entry.ImageURLs {
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
	}
	return nil
}

func outputJSON(results []ideas.SearchResult, total int) error {
	out := struct {
		Total   int               `json:"total"`
		Results []ideas.IdeaEntry `json:"results"`
	}{Total: total}
	for _, r := range results {
		out.Results = append(out.Results, r.Entry)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func runInit(d Deps) error {
	targetPath := dataSavePath(d.Cfg)
	if targetPath == "" {
		dir, err := ideasDir()
		if err != nil {
			return fmt.Errorf("cannot determine ideas data directory: %w", err)
		}
		targetPath = filepath.Join(dir, "ideas", "ideas.json")
	}

	if _, err := os.Stat(targetPath); err == nil {
		fmt.Fprintf(os.Stderr, "%s already exists.\n  To re-download the latest data, delete it first:\n    rm %s\n  Then run 'aigc-cli ideas init' again.\n", targetPath, targetPath)
		return fmt.Errorf("ideas data already exists")
	}

	fmt.Printf("Downloading ideas data from GitHub...\n")

	client := httpClient()

	resp, err := client.Get(dataURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d\n  URL: %s", resp.StatusCode, dataURL)
	}

	rawData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var entries []ideas.IdeaEntry
	if err := json.Unmarshal(rawData, &entries); err != nil {
		return fmt.Errorf("downloaded data is corrupted (invalid JSON): %w", err)
	}
	fmt.Printf("Downloaded %d prompt entries.\n", len(entries))

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(targetPath, rawData, 0644); err != nil {
		return fmt.Errorf("cannot save %s: %w", targetPath, err)
	}
	fmt.Printf("Saved to %s\n", targetPath)

	return nil
}

func httpClient() *http.Client {
	client := &http.Client{
		Timeout:   120 * time.Second,
		Transport: http.DefaultClient.Transport,
	}
	if client.Transport == nil {
		client.Transport = http.DefaultTransport
	}
	return client
}
