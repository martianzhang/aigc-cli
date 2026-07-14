package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/config"
	"github.com/martianzhang/apimart-cli/internal/service"
	"github.com/martianzhang/apimart-cli/internal/types"
)

// loadVideoDefaults returns the user's video config defaults.
// Tries shared.Cfg first (fast), falls back to reading from file.
func loadVideoDefaults() *types.VideoDefaults {
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Video != nil {
		return shared.Cfg.Defaults.Video
	}
	if cfg, err := config.Load(shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil {
		return cfg.Defaults.Video
	}
	return nil
}

// generateVideoAndSave generates videos via the configured provider and saves them to disk.
// Handles config merge, timeout, API dispatch, and download. Returns paths to saved files.
// Shared by CLI (video command) and agent loop (chat) — single source of truth.
// Supports APIMart async and OpenRouter video providers.
func generateVideoAndSave(c *client.Client, req *types.VideoGenerateRequest) ([]string, error) {
	// Always load the user's config — shared.Cfg may be nil if PersistentPreRunE hasn't run.
	vidCfg := loadVideoDefaults()

	// Check if LLM is allowed to override (default: false = config wins)
	allowOverride := false
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Chat != nil {
		allowOverride = shared.Cfg.Defaults.Chat.AllowToolOverride
	}

	if vidCfg != nil {
		if !allowOverride {
			if vidCfg.Model != "" {
				req.Model = vidCfg.Model
			}
			if vidCfg.Size != "" {
				req.Size = vidCfg.Size
			}
			if vidCfg.Resolution != "" {
				req.Resolution = vidCfg.Resolution
			}
			if vidCfg.Duration != nil {
				req.Duration = vidCfg.Duration
			}
		} else {
			vidCfg.MergeIntoVideo(req)
		}
	}
	// Code defaults for fields the user didn't configure
	if req.Size == "" {
		req.Size = "16:9"
	}
	if req.Resolution == "" {
		req.Resolution = "480p"
	}
	if req.Model == "" {
		return nil, fmt.Errorf("model is required: set via defaults.video.model in config.yaml")
	}

	// Set timeout
	applyTimeout(c, "video", client.VideoTimeout)

	// Dispatch based on provider
	if isOpenRouterProvider() {
		orReq := &types.OpenRouterVideoRequest{
			Model:  req.Model,
			Prompt: req.Prompt,
		}
		submitResp, err := c.OpenRouterVideoSubmit(orReq)
		if err != nil {
			return nil, fmt.Errorf("submission failed: %w", err)
		}
		pollResp, err := c.OpenRouterVideoPollUntilComplete(submitResp.PollingURL, 30*time.Second, 5*time.Minute)
		if err != nil {
			return nil, fmt.Errorf("polling failed: %w", err)
		}

		var saved []string
		for i, u := range pollResp.UnsignedURLs {
			filename := filepath.Join(shared.OutputDir, fmt.Sprintf("video_%s_%d.mp4", submitResp.ID, i))
			if err := service.SaveResource(u, filename); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
				continue
			}
			fmt.Printf("Saved: %s\n", filename)
			saved = append(saved, filename)
		}
		return saved, nil
	}

	// APIMart async
	resp, err := c.VideoSubmit(req)
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
	if taskData.Result != nil && len(taskData.Result.Videos) > 0 {
		saved, err := downloadVideos(taskData.Result.Videos, taskData.ID)
		if err != nil {
			return saved, err
		}
		return saved, nil
	}
	return nil, fmt.Errorf("no videos in task result")
}
