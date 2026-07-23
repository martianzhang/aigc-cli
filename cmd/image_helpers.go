package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// savePromptFile saves the generation prompt to image_{taskID}.md.
func savePromptFile(taskID, prompt string) {
	if !shared.SavePrompt {
		return
	}
	service.SavePrompt(shared.OutputDir, taskID, prompt)
}

// loadImageDefaults returns the user's image config defaults.
// Tries shared.Cfg first (fast), falls back to reading from file.
func loadImageDefaults() *types.ImageDefaults {
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Image != nil {
		return shared.Cfg.Defaults.Image
	}
	// Fallback: load from file directly
	if cfg, err := config.Load(shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil {
		return cfg.Defaults.Image
	}
	return nil
}

// generateImageAndSave generates images via the configured provider and saves them to disk.
// Handles config merge, timeout, API dispatch, and download. Returns paths to saved files.
// Shared by CLI (image command) and agent loop (chat) — single source of truth.
// Supports APIMart async and OpenAI-compatible sync providers.
func generateImageAndSave(c client.APIClient, req *types.GenerateRequest) ([]string, error) {
	// Always load the user's config — shared.Cfg may be nil if PersistentPreRunE
	// hasn't run (e.g., direct call from agent loop without CLI entry).
	imgCfg := loadImageDefaults()

	if shared.Verbose {
		if imgCfg != nil {
			fmt.Fprintf(os.Stderr, "\r\n[image] cfg: model=%s quality=%s size=%s res=%s\r\n",
				imgCfg.Model, imgCfg.Quality, imgCfg.Size, imgCfg.Resolution)
		} else {
			fmt.Fprintf(os.Stderr, "\r\n[image] WARNING: no image config loaded (check defaults.image in config.yaml)\r\n")
		}
		fmt.Fprintf(os.Stderr, "[image] req before: model=%s quality=%s size=%s res=%s\r\n",
			req.Model, req.Quality, req.Size, req.Resolution)
	}

	// Check if LLM is allowed to override config (default: false = config wins)
	allowOverride := false
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Chat != nil {
		allowOverride = shared.Cfg.Defaults.Chat.AllowToolOverride
	}

	if imgCfg != nil {
		if !allowOverride {
			// Config is the ceiling — force values regardless of LLM
			if imgCfg.Model != "" {
				req.Model = imgCfg.Model
			}
			if imgCfg.Quality != "" {
				req.Quality = imgCfg.Quality
			}
			if imgCfg.Size != "" {
				req.Size = imgCfg.Size
			}
			if imgCfg.Resolution != "" {
				req.Resolution = imgCfg.Resolution
			}
		} else {
			// allow_tool_override=true — LLM takes priority, config only fills empty fields
			imgCfg.MergeIntoImage(req)
		}
	}
	// Code defaults for fields the user didn't configure
	if req.Size == "" {
		req.Size = "1:1"
	}
	if req.Quality == "" {
		req.Quality = "low"
	}
	if req.Resolution == "" {
		req.Resolution = "1k"
	}
	if shared.Verbose {
		fmt.Fprintf(os.Stderr, "[image] req after:  model=%s quality=%s size=%s res=%s\r\n",
			req.Model, req.Quality, req.Size, req.Resolution)
	}
	if req.Model == "" {
		return nil, fmt.Errorf("model is required: set via defaults.image.model in config.yaml")
	}

	// Set timeout
	applyTimeout(c, "image", client.ImageTimeout)

	// Dispatch based on provider
	if isAPIMartProvider() {
		resp, err := c.Submit(req)
		if err != nil {
			return nil, fmt.Errorf("submission failed: %w", err)
		}
		if len(resp.Data) == 0 {
			return nil, fmt.Errorf("submission returned no tasks")
		}

		taskData, err := c.PollTask(resp.Data[0].TaskID)
		if err != nil {
			return nil, fmt.Errorf("polling failed: %w", err)
		}

		savePromptFile(taskData.ID, req.Prompt)
		if taskData.Result != nil && len(taskData.Result.Images) > 0 {
			saved, err := downloadImages(taskData.Result.Images, taskData.ID)
			postProcessImages(saved)
			if err != nil {
				return saved, err
			}
			return saved, nil
		}
		return nil, fmt.Errorf("no images in task result")
	}

	// OpenAI-compatible sync
	resp, err := c.ImageGenerateSync(req)
	if err != nil {
		return nil, fmt.Errorf("image generation failed: %w", err)
	}

	var saved []string
	for i, img := range resp.Data {
		if img.B64JSON != "" {
			taskID := fmt.Sprintf("image_sync_%d", resp.Created)
			filename, saveErr := service.SaveBase64Image(shared.OutputDir, taskID, img.B64JSON, i)
			if saveErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save image %d: %v\n", i, saveErr)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
			saved = append(saved, filename)
		} else if img.URL != "" {
			taskID := fmt.Sprintf("sync_%d", resp.Created)
			filename, saveErr := service.DownloadFile(img.URL, shared.OutputDir, fmt.Sprintf("image_%s_%d", taskID, i))
			if saveErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download image %d: %v\n", i, saveErr)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
			saved = append(saved, filename)
		}
	}
	postProcessImages(saved)
	if len(saved) == 0 {
		return nil, fmt.Errorf("no images saved")
	}
	return saved, nil
}

// postProcessImages applies --compress post-processing to already-saved images.
// Called after image generation/download from all 4 save points.
func postProcessImages(saved []string) {
	if genCompress == "" || len(saved) == 0 {
		return
	}
	targetSize, quality, err := service.ParseCompressOption(genCompress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: invalid --compress value %q: %v\n", genCompress, err)
		return
	}
	opts := &service.CompressOptions{
		TargetSize: targetSize,
		Quality:    quality,
		Format:     genOutputFormat,
	}
	for _, path := range saved {
		result, err := service.CompressImage(path, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: compress %s: %v\n", path, err)
			continue
		}
		if result.Skipped {
			fmt.Printf("  Compress %s: skipped (%s)\n", path, result.Reason)
		} else {
			savedStr := formatBytes(result.After)
			originalStr := formatBytes(result.Before)
			pct := 100 - int(float64(result.After)/float64(result.Before)*100)
			fmt.Printf("  Compress %s: %s → %s (%d%% saved)\n", path, originalStr, savedStr, pct)
		}
	}
}

// runLocalCompress implements the pure local compression mode (no API call).
// Compress --image-url files directly using --compress settings.
func runLocalCompress(compressVal string, imageURLs []string, outputFormat string) error {
	if len(imageURLs) == 0 {
		return fmt.Errorf("--image-url is required for local compression mode")
	}
	targetSize, quality, err := service.ParseCompressOption(compressVal)
	if err != nil {
		return fmt.Errorf("invalid --compress value %q: %w", compressVal, err)
	}
	opts := &service.CompressOptions{
		TargetSize: targetSize,
		Quality:    quality,
		Format:     outputFormat,
	}
	var results []*service.CompressResult
	for _, src := range imageURLs {
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			fmt.Fprintf(os.Stderr, "Warning: skipping remote URL (local files only): %s\n", src)
			continue
		}
		if !isFile(src) {
			fmt.Fprintf(os.Stderr, "Warning: file not found: %s\n", src)
			continue
		}
		result, err := service.CompressImage(src, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: compress %s: %v\n", src, err)
			continue
		}
		results = append(results, result)
	}
	if len(results) == 0 {
		return fmt.Errorf("no files were compressed")
	}
	fmt.Println("Compression results:")
	var totalBefore, totalAfter int64
	for _, r := range results {
		if r.Skipped {
			fmt.Printf("  %s: skipped (%s)\n", r.DstPath, r.Reason)
		} else {
			pct := 100 - int(float64(r.After)/float64(r.Before)*100)
			fmt.Printf("  %s: %s → %s (%d%% saved)\n", r.DstPath, formatBytes(r.Before), formatBytes(r.After), pct)
		}
		totalBefore += r.Before
		totalAfter += r.After
	}
	if totalBefore > 0 {
		pct := 100 - int(float64(totalAfter)/float64(totalBefore)*100)
		fmt.Printf("Total: %s → %s (%d%% saved)\n", formatBytes(totalBefore), formatBytes(totalAfter), pct)
	}
	return nil
}

// formatBytes returns a human-readable byte size string.
func formatBytes(b int64) string {
	switch {
	case b >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1fKB", float64(b)/1024)
	default:
		return fmt.Sprintf("%dB", b)
	}
}
