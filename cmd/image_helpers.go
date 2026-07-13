package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/config"
	"github.com/martianzhang/apimart-cli/internal/service"
	"github.com/martianzhang/apimart-cli/internal/types"
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
			body, getErr := httpGet(img.URL)
			if getErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download image %d: %v\n", i, getErr)
				continue
			}
			ext := filepath.Ext(img.URL)
			if ext == "" {
				ext = ".png"
			}
			taskID := fmt.Sprintf("sync_%d", resp.Created)
			filename := filepath.Join(shared.OutputDir, fmt.Sprintf("image_%s_%d%s", taskID, i, ext))
			if writeErr := os.WriteFile(filename, body, 0644); writeErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", filename, writeErr)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
			saved = append(saved, filename)
		}
	}
	if len(saved) == 0 {
		return nil, fmt.Errorf("no images saved")
	}
	return saved, nil
}
