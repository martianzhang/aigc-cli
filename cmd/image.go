package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/config"
	"github.com/martianzhang/apimart-cli/internal/types"
)

// Image-specific flag variables
var (
	genPrompt       string
	genSize         string
	genResolution   string
	genQuality      string
	genBackground   string
	genModeration   string
	genOutputFormat string
	genCompression  int
	genN            int
	genImageURLs    []string
	genMaskURL      string
	genStyle        string
	genResponseFmt  string
	genDryRun       bool
)

// imageCmd represents the `apimart-cli image` command.
var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "Generate images (supports OpenAI sync & APIMart async)",
	Long: `Generate images via any OpenAI-compatible API.

Supports text-to-image, image-to-image, and inpainting.
Works with OpenAI, OpenRouter (sync), and APIMart (async task-based).

You can specify parameters via flags, or pass a complete JSON request
via the --json flag (file path, JSON string, or "-" for stdin).

Examples:
  apimart-cli image --prompt "A cat under starry sky"
  apimart-cli image --prompt prompt.txt --size "16:9"
  echo "..." | apimart-cli image --prompt -
  apimart-cli image --json request.json
  apimart-cli image --json '{"prompt":"a red fox","n":4}'`,
	RunE: runImageGenerate,
}

func runImageGenerate(cmd *cobra.Command, args []string) error {
	// ----- Step 1: Build the request -----
	req, err := buildImageRequest(cmd)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}

	// ----- Step 2: Merge config defaults -----
	if cfg, err := config.LoadDefaults(cfgFile); err == nil && cfg != nil && cfg.Defaults != nil {
		cfg.Defaults.Image.MergeIntoImage(req)
	}

	// ----- Step 3: Apply defaults for remaining empty fields -----
	if req.Model == "" {
		return fmt.Errorf("model is required: set via --model flag or defaults.image.model in config.yaml")
	}
	if req.Size == "" {
		req.Size = "1:1"
	}
	if req.Quality == "" {
		req.Quality = "auto"
	}
	if req.OutputFormat == "" {
		req.OutputFormat = "png"
	}

	if genDryRun {
		curl := buildImageCurl(req)
		fmt.Println(curl)
		return nil
	}

	// ----- Step 4: Print the request payload (verbose only) -----
	if verbose {
		prettyReq, _ := json.MarshalIndent(req, "", "  ")
		fmt.Printf("Request:\n%s\n\n", string(prettyReq))
	}

	// ----- Step 5: Resolve local image files (upload if needed) -----
	c := client.New(apiKey, apiBase, httpProxy)
	isAsync := isAPIMartProvider()
	if isAsync {
		if len(req.ImageURLs) > 0 {
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return fmt.Errorf("failed to resolve image-urls: %w", err)
			}
			req.ImageURLs = resolved
		}
		if req.MaskURL != "" {
			resolved, err := c.ResolveLocalImages([]string{req.MaskURL})
			if err != nil {
				return fmt.Errorf("failed to resolve mask-url: %w", err)
			}
			req.MaskURL = resolved[0]
		}
	}

	if isAsync {
		return runAsyncImage(c, req)
	}
	return runSyncImage(c, req)
}

// runSyncImage handles OpenAI/OpenRouter-compatible synchronous image generation.
func runSyncImage(c *client.Client, req *types.GenerateRequest) error {
	syncResp, err := c.ImageGenerateSync(req)
	if err != nil {
		return fmt.Errorf("image generation failed: %w", err)
	}

	fmt.Printf("Created: %d\n", syncResp.Created)
	for i, img := range syncResp.Data {
		url := img.URL
		if url == "" && img.B64JSON != "" {
			url = "<base64 data>"
		}
		fmt.Printf("Image %d: %s\n", i+1, url)
		if img.RevisedPrompt != "" {
			fmt.Printf("  Revised prompt: %s\n", img.RevisedPrompt)
		}
		// Download if URL is present
		if img.URL != "" {
			body, err := httpGet(img.URL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download image %d: %v\n", i, err)
				continue
			}
			ext := filepath.Ext(img.URL)
			if ext == "" {
				ext = ".png"
			}
			taskID := fmt.Sprintf("sync_%d", syncResp.Created)
			filename := filepath.Join(outputDir, fmt.Sprintf("image_%s_%d%s", taskID, i, ext))
			if err := os.WriteFile(filename, body, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", filename, err)
				continue
			}
			fmt.Printf("Saved: %s\n", filename)
		}
	}
	return nil
}

// runAsyncImage handles APIMart-compatible asynchronous (task-based) image generation.
func runAsyncImage(c *client.Client, req *types.GenerateRequest) error {
	resp, err := c.Submit(req)
	if err != nil {
		return fmt.Errorf("submission failed: %w", err)
	}

	if len(resp.Data) == 0 {
		return fmt.Errorf("submission returned no tasks")
	}

	task := resp.Data[0]
	fmt.Printf("Response code: %d\n", resp.Code)
	fmt.Printf("Task ID: %s\n", task.TaskID)
	fmt.Printf("Status: %s\n\n", task.Status)

	fmt.Println("Polling for completion...")
	taskData, err := c.PollTask(task.TaskID)
	if err != nil {
		return fmt.Errorf("polling failed: %w", err)
	}

	if verbose {
		prettyResult, _ := json.MarshalIndent(taskData, "", "  ")
		fmt.Printf("\nTask result:\n%s\n", string(prettyResult))
	}

	fmt.Println()
	savePromptFile(taskData.ID, req.Prompt)

	if taskData.Result != nil && len(taskData.Result.Images) > 0 {
		if err := downloadImages(taskData.Result.Images, taskData.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
		}
	}

	fmt.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
		taskData.ActualTime, taskData.Cost, taskData.CreditsCost)
	return nil
}

// isAPIMartProvider determines whether to use APIMart async mode.
// Known APIMart domains: apimart.ai, apib.ai, aiuxu.com, aishuch.com
// Known sync domains: openai.com, openrouter.ai
// All other domains default to sync (OpenAI-compatible relay).
func isAPIMartProvider() bool {
	switch mode {
	case "async":
		return true
	case "sync":
		return false
	default: // auto — detect from base URL
		base := apiBase
		if base == "" {
			base = "https://api.apimart.ai"
		}
		// Check against known APIMart async domains
		apimartDomains := []string{"apimart.ai", "apib.ai", "aiuxu.com", "aishuch.com"}
		for _, d := range apimartDomains {
			if strings.Contains(base, d) {
				return true
			}
		}
		// Everything else (openai.com, openrouter.ai, or any relay) → sync
		return false
	}
}

// buildImageRequest constructs a GenerateRequest from --json or individual flags.
func buildImageRequest(cmd *cobra.Command) (*types.GenerateRequest, error) {
	if jsonInput != "" {
		return parseJSONInput()
	}

	prompt, err := resolvePrompt()
	if err != nil {
		return nil, err
	}

	req := &types.GenerateRequest{
		Model:          model,
		Prompt:         prompt,
		Size:           genSize,
		Resolution:     genResolution,
		Quality:        genQuality,
		Background:     genBackground,
		Moderation:     genModeration,
		OutputFormat:   genOutputFormat,
		ImageURLs:      genImageURLs,
		MaskURL:        genMaskURL,
		Style:          genStyle,
		ResponseFormat: genResponseFmt,
	}

	if cmd.Flags().Changed("output-compression") {
		v := genCompression
		req.OutputCompression = &v
	}
	if cmd.Flags().Changed("n") {
		v := genN
		req.N = &v
	}

	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required (use --prompt or --json)")
	}

	return req, nil
}

// buildImageCurl generates an equivalent curl command for an image generation request.
func buildImageCurl(req *types.GenerateRequest) string {
	body, _ := json.Marshal(req)
	base := apiBase
	if base == "" {
		base = "https://api.apimart.ai/v1" // matches client.defaultBaseURL
	}
	base = strings.TrimRight(base, "/")
	url := base + "/images/generations"

	cmd := fmt.Sprintf("curl -X POST %s \\\n", url)
	cmd += fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", apiKey)
	cmd += "  -H \"Content-Type: application/json\" \\\n"
	cmd += fmt.Sprintf("  -d '%s'", string(body))
	return cmd
}

// registerImageGenerateFlags adds the image generation flags to a command.
func registerImageGenerateFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVarP(&genPrompt, "prompt", "p", "", "Text description (auto-reads from file if path exists, or \"-\" for stdin)")
	f.StringVarP(&genSize, "size", "s", "", `Aspect ratio (e.g. "16:9", "1:1") or pixel dims (e.g. "1024x1024")`)
	f.StringVarP(&genResolution, "resolution", "r", "", "Resolution tier: 1k, 2k, 4k (APIMart only)")
	f.StringVarP(&genQuality, "quality", "q", "", "Quality: auto, low, medium, high")
	f.StringVar(&genBackground, "background", "", "Background mode: auto, opaque, transparent (APIMart only)")
	f.StringVar(&genModeration, "moderation", "", "Moderation strength: auto, low (APIMart only)")
	f.StringVarP(&genOutputFormat, "output-format", "f", "", "Output format: png, jpeg, webp")
	f.IntVar(&genCompression, "output-compression", 0, "Output compression level 0-100 (jpeg/webp only) (APIMart only)")
	f.IntVar(&genN, "n", 0, "Number of images to generate (1-4)")
	f.StringArrayVar(&genImageURLs, "image-url", nil, "Reference image URL (repeatable) (APIMart only)")
	f.StringVar(&genMaskURL, "mask-url", "", "Mask image URL for inpainting (APIMart only)")
	f.StringVar(&genStyle, "style", "", "Image style: vivid, natural (OpenAI only)")
	f.StringVar(&genResponseFmt, "response-format", "", "Response format: url, b64_json (OpenAI/OpenRouter)")
	f.BoolVar(&genDryRun, "dry-run", false, "Print request parameters without calling API")
	f.StringVar(&jsonInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	f.StringVar(&mode, "mode", "", "Generation mode: auto (detect), sync, async (default: auto)")
	f.BoolVar(&savePrompt, "save-prompt", false, "save prompt to .md file alongside results")
}

func init() {
	registerImageGenerateFlags(imageCmd)
	rootCmd.AddCommand(imageCmd)
}

// The following helpers are shared across image commands and aliases.

// resolvePrompt resolves the prompt text from --prompt flag.
// Defaults to stdin when --prompt is not specified.
func resolvePrompt() (string, error) {
	input := genPrompt
	if input == "" {
		input = "-"
	}
	if input == "-" || isFile(input) {
		data, err := readInput(input)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt: %w", err)
		}
		return string(data), nil
	}
	return input, nil
}

// readInput reads content from a file path, stdin ("-"), or returns the raw string.
func readInput(input string) ([]byte, error) {
	switch {
	case input == "-":
		return io.ReadAll(os.Stdin)
	case isFile(input):
		return os.ReadFile(input)
	default:
		return []byte(input), nil
	}
}

// parseJSONInput reads JSON from file path, string literal, or stdin.
func parseJSONInput() (*types.GenerateRequest, error) {
	data, err := readInput(jsonInput)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON input: %w", err)
	}

	req := &types.GenerateRequest{}
	if err := json.Unmarshal(data, req); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required in JSON input")
	}

	return req, nil
}

// isFile returns true if the given path points to an existing file.
func isFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// downloadImages downloads all generated images to the output directory.
func downloadImages(images []types.ImageResult, taskID string) error {
	for i, img := range images {
		for j, url := range img.URL {
			resp, err := httpGet(url)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download image %d-%d: %v\n", i, j, err)
				continue
			}

			ext := filepath.Ext(url)
			if ext == "" {
				ext = ".png"
			}
			filename := filepath.Join(outputDir, fmt.Sprintf("image_%s_%d_%d%s", taskID, i, j, ext))
			if err := os.WriteFile(filename, resp, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", filename, err)
				continue
			}
			fmt.Printf("Saved: %s\n", filename)
		}
	}
	return nil
}

// savePromptFile saves the generation prompt to apimart_{taskID}.md.
func savePromptFile(taskID, prompt string) {
	if !savePrompt || prompt == "" {
		return
	}
	filename := filepath.Join(outputDir, fmt.Sprintf("image_%s.md", taskID))
	content := fmt.Sprintf("# %s\n\n%s\n", taskID, prompt)
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save prompt file: %v\n", err)
	}
	fmt.Printf("Prompt saved: %s\n", filename)
}

// httpGet performs a simple GET request and returns the body bytes.
func httpGet(url string) ([]byte, error) {
	resp, err := http.DefaultClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
