package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/config"
	"github.com/martianzhang/apimart-cli/internal/provider"
	"github.com/martianzhang/apimart-cli/internal/service"
	"github.com/martianzhang/apimart-cli/internal/types"
)

// readInput reads content from a file path, stdin ("-"), or returns the raw string.
func readInput(input string) ([]byte, error) {
	switch {
	case input == "-":
		// Don't block if stdin is a terminal with no piped input
		stat, err := os.Stdin.Stat()
		if err == nil && (stat.Mode()&os.ModeCharDevice) != 0 {
			return nil, fmt.Errorf("stdin is a terminal — pipe input or use --prompt")
		}
		return io.ReadAll(os.Stdin)
	case isFile(input):
		return os.ReadFile(input)
	default:
		return []byte(input), nil
	}
}

// isFile returns true if the given path points to an existing file.
func isFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// httpGet performs an HTTP GET or resolves a data URI / base64 string.
func httpGet(rawURL string) ([]byte, error) {
	return service.FetchImage(rawURL)
}

// applyTimeout sets the HTTP client timeout from CLI flag / config, falling back to modDefault.
// Priority: --timeout flag > defaults.{mod}.timeout > timeout > modDefault.
func applyTimeout(c client.APIClient, modKey string, modDefault time.Duration) {
	d := modDefault
	// 1. CLI --timeout flag (global override)
	if shared.TimeoutFlag > 0 {
		d = time.Duration(shared.TimeoutFlag) * time.Second
		c.SetTimeout(d)
		return
	}
	// 2. Config file
	if cfg, err := config.LoadDefaults(shared.CfgFile); err == nil && cfg != nil {
		var modTimeout *int
		if cfg.Defaults != nil {
			switch modKey {
			case "image":
				if cfg.Defaults.Image != nil {
					modTimeout = cfg.Defaults.Image.Timeout
				}
			case "video":
				if cfg.Defaults.Video != nil {
					modTimeout = cfg.Defaults.Video.Timeout
				}
			case "midjourney":
				if cfg.Defaults.Midjourney != nil {
					modTimeout = cfg.Defaults.Midjourney.Timeout
				}
			}
		}
		if modTimeout != nil && *modTimeout > 0 {
			d = time.Duration(*modTimeout) * time.Second
		} else if cfg.Timeout != nil && *cfg.Timeout > 0 {
			d = time.Duration(*cfg.Timeout) * time.Second
		}
	}
	c.SetTimeout(d)
}

// isOpenRouterProvider determines whether the current base URL points to OpenRouter.
func isOpenRouterProvider() bool {
	return provider.IsOpenRouter(shared.APIBase)
}

// isAPIMartProvider determines whether to use APIMart async mode.
// Known APIMart domains: apimart.ai, apib.ai, aiuxu.com, aishuch.com
// Known sync domains: openai.com, openrouter.ai
// All other domains default to sync (OpenAI-compatible relay).
func isAPIMartProvider() bool {
	switch shared.Mode {
	case "async":
		return true
	case "sync":
		return false
	default: // auto -- detect from base URL
		base := shared.APIBase
		if base == "" {
			base = "https://api.apimart.ai"
		}
		return provider.IsAPIMart(base)
	}
}

// setIntFlag sets a *int field from a cobra flag if changed.
func setIntFlag(cmd *cobra.Command, name string, target **int, val int) {
	if cmd.Flags().Changed(name) {
		v := val
		*target = &v
	}
}

// setBoolFlag sets a *bool field from a cobra flag if changed.
func setBoolFlag(cmd *cobra.Command, name string, target **bool, val bool) {
	if cmd.Flags().Changed(name) {
		v := val
		*target = &v
	}
}

// setFloatFlag sets a *float64 field from a cobra flag if changed.
func setFloatFlag(cmd *cobra.Command, name string, target **float64, val float64) {
	if cmd.Flags().Changed(name) {
		v := val
		*target = &v
	}
}

// extractExt returns the file extension from a URL, defaulting to .mp4.
func extractExt(rawURL string) string {
	return service.ExtractExt(rawURL)
}

// downloadVideos downloads all generated videos. Returns paths to saved files.
func downloadVideos(videos []types.VideoResult, taskID string) ([]string, error) {
	var saved []string
	for i, vid := range videos {
		for j, url := range vid.URL {
			ext := extractExt(url)
			filename := filepath.Join(shared.OutputDir, fmt.Sprintf("video_%s_%d_%d%s", taskID, i, j, ext))
			if err := client.DownloadFile(http.DefaultClient, url, filename); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download video %d-%d: %v\n", i, j, err)
				continue
			}
			fmt.Printf("Saved: %s\n", filename)
			saved = append(saved, filename)
		}
	}
	return saved, nil
}

// saveImage downloads a single image from a URL and saves it to disk.
func saveImage(imageURL, taskID string, index int) (string, error) {
	body, err := httpGet(imageURL)
	if err != nil {
		return "", err
	}
	ext := filepath.Ext(imageURL)
	if ext == "" {
		ext = ".png"
	}
	filename := filepath.Join(shared.OutputDir, fmt.Sprintf("image_%s_%d%s", taskID, index, ext))
	if err := os.WriteFile(filename, body, 0644); err != nil {
		return "", fmt.Errorf("failed to save %s: %w", filename, err)
	}
	return filename, nil
}

// printUsage prints token usage and cost information.
func printUsage(usage *types.OpenAIImageUsage) {
	if usage == nil {
		return
	}
	var parts []string
	if usage.PromptTokens > 0 {
		parts = append(parts, fmt.Sprintf("%d in", usage.PromptTokens))
	}
	if usage.CompletionTokens > 0 {
		parts = append(parts, fmt.Sprintf("%d out", usage.CompletionTokens))
	}
	if usage.TotalTokens > 0 {
		parts = append(parts, fmt.Sprintf("%d total", usage.TotalTokens))
	}
	tokenStr := ""
	if len(parts) > 0 {
		tokenStr = strings.Join(parts, " / ")
	}
	if tokenStr != "" || usage.Cost > 0 {
		if tokenStr != "" {
			fmt.Printf("Tokens: %s", tokenStr)
		}
		if usage.Cost > 0 {
			if tokenStr != "" {
				fmt.Printf(" | ")
			}
			fmt.Printf("Cost: $%.5f", usage.Cost)
		}
		fmt.Println()
	}
}
