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

// generate flags
var (
	genModel        string
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
	genDryRun       bool
)

// generateCmd represents the `generate` subcommand.
var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate images via the APIMart API",
	Long: `Generate images using the GPT-Image-2 model.

You can specify parameters via individual flags, or pass a complete JSON
request via the --json flag (file path, JSON string, or "-" for stdin).
If --prompt points to an existing file, its content is read automatically.
When both flags and --json are provided, --json takes precedence.

Examples:
  apimart-cli generate --prompt "A cat under starry sky"
  apimart-cli generate --prompt prompt.txt --size "16:9"
  echo "A detailed cyberpunk cityscape" | apimart-cli generate --prompt -
  apimart-cli generate --json request.json
  apimart-cli generate --json '{"prompt":"a red fox","n":4}'
  cat request.json | apimart-cli generate --json -`,
	RunE: runGenerate,
}

func runGenerate(cmd *cobra.Command, args []string) error {
	// ----- Step 1: Build the request -----
	req, err := buildRequest(cmd)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}

	// ----- Step 2: Merge config defaults -----
	defaults, err := config.LoadDefaults(cfgFile)
	if err != nil {
		// Non-fatal: config may not exist
		fmt.Fprintf(os.Stderr, "Note: could not load config defaults: %v\n", err)
	}
	defaults.MergeInto(req)

	// ----- Step 3: Apply hard-coded defaults for remaining empty fields -----
	if req.Model == "" {
		req.Model = "gpt-image-2-official"
	}
	if req.Size == "" {
		req.Size = "1:1"
	}
	if req.Resolution == "" {
		req.Resolution = "1k"
	}
	if req.Quality == "" {
		req.Quality = "auto"
	}
	if req.OutputFormat == "" {
		req.OutputFormat = "png"
	}

	if genDryRun {
		curl := buildCurl(req)
		fmt.Println(curl)
		return nil
	}

	// ----- Step 4: Print the request payload -----
	prettyReq, _ := json.MarshalIndent(req, "", "  ")
	fmt.Printf("Request:\n%s\n\n", string(prettyReq))

	// ----- Step 5: Submit -----
	c := client.New(apiKey, apiBase, httpProxy)
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

	// ----- Step 7: Print result -----
	prettyResult, _ := json.MarshalIndent(taskData, "", "  ")
	fmt.Printf("\nTask result:\n%s\n", string(prettyResult))

	// ----- Step 8: Download images (optional) -----
	if taskData.Result != nil && len(taskData.Result.Images) > 0 {
		return downloadImages(taskData.Result.Images)
	}

	return nil
}

// buildRequest constructs a GenerateRequest from --json or individual flags.
func buildRequest(cmd *cobra.Command) (*types.GenerateRequest, error) {
	// If --json is provided, parse it
	if jsonInput != "" {
		return parseJSONInput()
	}

	// Resolve prompt from --prompt or --prompt-file
	prompt, err := resolvePrompt()
	if err != nil {
		return nil, err
	}

	// Build from flags
	req := &types.GenerateRequest{
		Model:        genModel,
		Prompt:       prompt,
		Size:         genSize,
		Resolution:   genResolution,
		Quality:      genQuality,
		Background:   genBackground,
		Moderation:   genModeration,
		OutputFormat: genOutputFormat,
		ImageURLs:    genImageURLs,
		MaskURL:      genMaskURL,
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

// resolvePrompt resolves the prompt text.
// If --prompt is "-", reads from stdin.
// If --prompt is an existing file, reads its content.
// Otherwise uses --prompt as the literal prompt text.
func resolvePrompt() (string, error) {
	if genPrompt == "" {
		return "", nil
	}
	// Autodetect: stdin, file, or literal text
	if genPrompt == "-" || isFile(genPrompt) {
		data, err := readInput(genPrompt)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt: %w", err)
		}
		return string(data), nil
	}
	return genPrompt, nil
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
func downloadImages(images []types.ImageResult) error {
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
			filename := filepath.Join(outputDir, fmt.Sprintf("apimart_%s_task_%d_%d%s", "generate", i, j, ext))
			if err := os.WriteFile(filename, resp, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", filename, err)
				continue
			}
			fmt.Printf("Saved: %s\n", filename)
		}
	}
	return nil
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

// buildCurl generates an equivalent curl command for the request.
func buildCurl(req *types.GenerateRequest) string {
	body, _ := json.Marshal(req)
	base := apiBase
	if base == "" {
		base = "https://api.apimart.ai"
	}
	// Remove trailing slash
	base = strings.TrimRight(base, "/")
	url := base + "/v1/images/generations"

	// Build the curl command with -q to skip .curlrc
	cmd := fmt.Sprintf("curl -X POST %s \\\n", url)
	cmd += fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", apiKey)
	cmd += "  -H \"Content-Type: application/json\" \\\n"
	cmd += fmt.Sprintf("  -d '%s'", string(body))
	return cmd
}

func init() {
	rootCmd.AddCommand(generateCmd)

	f := generateCmd.Flags()
	f.StringVar(&genModel, "model", "", `Model name (default "gpt-image-2-official")`)
	f.StringVar(&genPrompt, "prompt", "", "Text description (auto-reads from file if path exists, or \"-\" for stdin)")
	f.StringVar(&genSize, "size", "", `Aspect ratio (e.g. "16:9", "1:1") or pixel dims (e.g. "1024x1024")`)
	f.StringVar(&genResolution, "resolution", "", "Resolution tier: 1k, 2k, 4k")
	f.StringVar(&genQuality, "quality", "", "Quality: auto, low, medium, high")
	f.StringVar(&genBackground, "background", "", "Background mode: auto, opaque, transparent")
	f.StringVar(&genModeration, "moderation", "", "Moderation strength: auto, low")
	f.StringVar(&genOutputFormat, "output-format", "", "Output format: png, jpeg, webp")
	f.IntVar(&genCompression, "output-compression", 0, "Output compression level 0-100 (jpeg/webp only)")
	f.IntVar(&genN, "n", 0, "Number of images to generate (1-4)")
	f.StringArrayVar(&genImageURLs, "image-url", nil, "Reference image URL (repeatable)")
	f.StringVar(&genMaskURL, "mask-url", "", "Mask image URL for inpainting")
	f.BoolVar(&genDryRun, "dry-run", false, "Print request parameters without calling API")

	// Note: flag .Changed is checked at runtime inside buildRequest
}
