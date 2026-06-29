package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Image2Studio web search base URL
const ideasWebBase = "https://image2studio.com/prompts"

// Image2Studio API base URL
const ideasAPIBase = "https://api.image2studio.com/public/prompts/search"

const ideasDefaultLimit = 8
const ideasDefaultPageSize = 8

// ideas flag variables
var (
	ideasLimit      int
	ideasPage       int
	ideasPageSize   int
	ideasCategory   string
	ideasFeatured   bool
	ideasJSON       bool
	ideasSaveImages bool
)

// --- API response types ---

type ideasResponse struct {
	OK   bool      `json:"ok"`
	Data ideasData `json:"data"`
}

type ideasData struct {
	Results []ideasResult `json:"results"`
	Total   int           `json:"total"`
}

type ideasResult struct {
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Prompt      string      `json:"prompt"`
	Model       string      `json:"model"`
	Categories  []ideasCat  `json:"categories"`
	ImageURL    string      `json:"imageUrl"`
	Image       ideasImage  `json:"image"`
	Source      ideasSource `json:"source"`
	StudioURL   string      `json:"studioUrl"`
	DetailURL   string      `json:"detailUrl"`
}

type ideasCat struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

type ideasImage struct {
	URL      string `json:"url"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	MimeType string `json:"mimeType"`
}

type ideasSource struct {
	AuthorName string `json:"authorName"`
	AuthorURL  string `json:"authorUrl"`
	Platform   string `json:"platform"`
}

// ideasCmd represents the `apimart-cli ideas` command.
var ideasCmd = &cobra.Command{
	Use:          "ideas [keywords]",
	Short:        "Search AI image prompt ideas from Image2Studio",
	SilenceUsage: true,
	Long: `Search AI image generation prompt ideas from Image2Studio's public prompt library.

Outputs markdown by default, with each result as a section containing
description, reference image, full prompt text, and metadata.

Keywords can be passed as arguments or via stdin (use "-" for stdin,
or pipe input):

  apimart-cli ideas "cinematic portrait"
  apimart-cli ideas "luxury perfume" --limit 3
  echo "cyberpunk city" | apimart-cli ideas
  apimart-cli ideas --json "cat" | jq '.results[].prompt'`,
	RunE: runIdeas,
}

func runIdeas(cmd *cobra.Command, args []string) error {
	// Resolve keywords: args → stdin
	keywords, err := resolveIdeasKeywords(args)
	if err != nil {
		return err
	}
	if keywords == "" {
		return fmt.Errorf("keywords are required: pass as argument or pipe to stdin")
	}

	// Validate flags: --page and --limit are mutually exclusive
	hasPage := cmd.Flags().Changed("page")
	hasLimit := cmd.Flags().Changed("limit")
	if hasPage && hasLimit {
		return fmt.Errorf("--page and --limit cannot be used together")
	}

	// Determine API pagination params
	apiPage := 1
	apiLimit := ideasLimit
	if hasPage {
		apiPage = ideasPage
		apiLimit = ideasPageSize
	}

	// Build search URL
	u, err := url.Parse(ideasAPIBase)
	if err != nil {
		return fmt.Errorf("invalid API base URL: %w", err)
	}
	q := u.Query()
	q.Set("q", keywords)
	q.Set("limit", fmt.Sprintf("%d", apiLimit))
	q.Set("page", fmt.Sprintf("%d", apiPage))
	if ideasCategory != "" {
		q.Set("category", ideasCategory)
	}
	if ideasFeatured {
		q.Set("featured", "true")
	}
	u.RawQuery = q.Encode()

	// Build web search URL for the header link
	webURL, _ := url.Parse(ideasWebBase)
	wq := webURL.Query()
	wq.Set("q", keywords)
	webURL.RawQuery = wq.Encode()

	// Fetch
	resp, err := http.DefaultClient.Get(u.String())
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result ideasResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("API returned error")
	}

	if len(result.Data.Results) == 0 {
		fmt.Println("没有找到匹配的提示词。")
		return nil
	}

	// Save images if requested
	if ideasSaveImages {
		if err := saveResultImages(result.Data.Results); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save some images: %v\n", err)
		}
	}

	// Output
	if ideasJSON {
		return outputIdeasJSON(result.Data)
	}
	return outputIdeasMarkdown(result.Data, keywords, webURL.String())
}

// resolveIdeasKeywords reads keywords from args or stdin.
func resolveIdeasKeywords(args []string) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}

	// Check if stdin is piped
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", nil
	}
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		// Terminal input, not piped — no keywords
		return "", nil
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("failed to read stdin: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// outputIdeasMarkdown prints search results in markdown format.
func outputIdeasMarkdown(data ideasData, keywords string, webURL string) error {
	now := time.Now().Format("2006-01-02")
	fmt.Printf("# Ideas: %s\n", keywords)
	fmt.Printf("> 找到 %d 个结果 · %s · [在线浏览](%s)\n\n", data.Total, now, webURL)

	for i, r := range data.Results {
		if i > 0 {
			fmt.Println("---")
			fmt.Println()
		}

		// Title
		title := r.Title
		if title == "" {
			title = fmt.Sprintf("Result %d", i+1)
		}
		fmt.Printf("## %s\n\n", title)

		// Description
		if r.Description != "" {
			fmt.Printf("%s\n\n", r.Description)
		}

		// Image
		imgURL := r.ImageURL
		if imgURL == "" {
			imgURL = r.Image.URL
		}
		if ideasSaveImages {
			localPath := localImagePath(i, imgURL)
			if localPath != "" {
				fmt.Printf("![参考图](%s)\n\n", localPath)
			}
		} else if imgURL != "" {
			fmt.Printf("![参考图](%s)\n\n", imgURL)
		}

		// Prompt
		fmt.Printf("**提示词：**\n```\n%s\n```\n\n", r.Prompt)

		// Categories
		if len(r.Categories) > 0 {
			var tags []string
			for _, c := range r.Categories {
				tags = append(tags, fmt.Sprintf("`#%s`", c.Slug))
			}
			fmt.Printf("%s\n\n", strings.Join(tags, " "))
		}

		// Model & metadata
		var meta []string
		if r.Model != "" {
			meta = append(meta, fmt.Sprintf("模型: %s", r.Model))
		}
		if r.Source.AuthorName != "" {
			meta = append(meta, fmt.Sprintf("作者: %s", r.Source.AuthorName))
		}
		if r.DetailURL != "" {
			meta = append(meta, fmt.Sprintf("[详情](%s)", r.DetailURL))
		}
		if len(meta) > 0 {
			fmt.Printf("%s\n\n", strings.Join(meta, " · "))
		}
	}
	return nil
}

// outputIdeasJSON prints search results as JSON.
func outputIdeasJSON(data ideasData) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	// Build a clean output structure
	out := struct {
		Total   int           `json:"total"`
		Results []ideasResult `json:"results"`
	}{
		Total:   data.Total,
		Results: data.Results,
	}
	return enc.Encode(out)
}

// saveResultImages downloads reference images to {output_dir}/ideas/images/.
func saveResultImages(results []ideasResult) error {
	dir := filepath.Join(shared.OutputDir, "ideas", "images")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create images directory: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	for i, r := range results {
		imgURL := r.ImageURL
		if imgURL == "" {
			imgURL = r.Image.URL
		}
		if imgURL == "" {
			continue
		}

		ext := filepath.Ext(imgURL)
		if ext == "" {
			ext = ".jpg"
		}
		filename := filepath.Join(dir, fmt.Sprintf("result-%d%s", i+1, ext))

		resp, err := client.Get(imgURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to download image %d: %v\n", i+1, err)
			continue
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to read image %d: %v\n", i+1, err)
			continue
		}
		if err := os.WriteFile(filename, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", filename, err)
			continue
		}
	}
	return nil
}

// localImagePath returns the local path for a saved image, or "" if not saved.
func localImagePath(index int, remoteURL string) string {
	if remoteURL == "" {
		return ""
	}
	ext := filepath.Ext(remoteURL)
	if ext == "" {
		ext = ".jpg"
	}
	return filepath.Join("ideas", "images", fmt.Sprintf("result-%d%s", index+1, ext))
}

func init() {
	f := ideasCmd.Flags()
	f.IntVarP(&ideasLimit, "limit", "l", ideasDefaultLimit, "Number of results (simple mode, 1-based, mutually exclusive with --page)")
	f.IntVarP(&ideasPage, "page", "p", 1, "Page number (pagination mode, 1-based, mutually exclusive with --limit)")
	f.IntVar(&ideasPageSize, "page-size", ideasDefaultPageSize, "Results per page when using --page (default 8, max 20)")
	f.StringVar(&ideasCategory, "category", "", "Filter by category slug (e.g. photography, portrait-selfie)")
	f.BoolVar(&ideasFeatured, "featured", false, "Show only featured prompts")
	f.BoolVar(&ideasJSON, "json", false, "Output as JSON instead of markdown")
	f.BoolVar(&ideasSaveImages, "save", false, "Download reference images to local directory")

	rootCmd.AddCommand(ideasCmd)
}
