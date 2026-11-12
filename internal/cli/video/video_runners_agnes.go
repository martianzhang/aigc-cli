package video

import (
	"context"
	"fmt"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/clientfactory"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/ui"
	"github.com/martianzhang/aigc-cli/internal/wait"
)

// runAgnesVideo handles video generation via agnes.ai's async task API.
// Uses POST /v1/videos for submission and GET /agnesapi?video_id= for polling.
// agnes has no upload endpoint, so buildVideoPlan embeds local files as data URIs.
func runAgnesVideo(ctx context.Context, c *client.Client, p *provider.EffectiveProvider, req *types.VideoGenerateRequest) (*RunResult, error) {
	// Step 1: Submit
	createResp, err := c.AgnesVideoSubmit(req)
	if err != nil {
		return nil, fmt.Errorf("agnes video submission failed: %w", err)
	}

	videoID := createResp.VideoID
	if videoID == "" {
		videoID = createResp.TaskID
	}
	if videoID == "" {
		return nil, fmt.Errorf("agnes video submission returned no video id")
	}

	ui.Printf("Provider: %s\n", p.ProviderType)
	ui.Printf("Model: %s\n", req.Model)
	ui.Printf("Task ID: %s\n", createResp.TaskID)
	ui.Printf("Status: %s\n\n", createResp.Status)

	// Step 2: Poll
	ui.Println("Polling for completion...")
	agnesMaxWait := clientfactory.TaskWait(types.MediaVideo)
	start := time.Now()
	var videoURL string
	for {
		if time.Since(start) > agnesMaxWait {
			return nil, fmt.Errorf("agnes video polling timed out after %v", agnesMaxWait)
		}

		queryResp, err := c.AgnesVideoQuery(videoID, req.Model)
		if err != nil {
			return nil, fmt.Errorf("polling failed: %w", err)
		}

		switch queryResp.Status {
		case "completed", "succeeded", "success":
			videoURL = queryResp.URL
			if videoURL == "" && queryResp.Metadata != nil {
				videoURL = queryResp.Metadata.URL
			}
			if videoURL == "" {
				return nil, fmt.Errorf("agnes video completed but no url returned")
			}
		case "failed", "failure":
			return nil, fmt.Errorf("agnes video generation failed: status=%s", queryResp.Status)
		case "cancelled", "expired":
			return nil, fmt.Errorf("agnes video generation %s", queryResp.Status)
		default:
			// queued / pending / in_progress -- keep waiting
			progress := fmt.Sprintf("%.0fs", time.Since(start).Seconds())
			ui.Printf("  Status: %s, Elapsed: %s\n", queryResp.Status, progress)
			if err := wait.Sleep(ctx, client.DefaultPollInterval); err != nil {
				return nil, err
			}
		}

		if videoURL != "" {
			break
		}
	}

	// Step 3: Download
	ui.Println()
	ui.Printf("Downloading video...\n")
	savePromptFile(videoID, req.Prompt)
	filename, err := service.DownloadFile(videoURL, options.Shared.OutputDir, fmt.Sprintf("video_agnes_%s", videoID))
	if err != nil {
		return nil, fmt.Errorf("failed to download video: %w", err)
	}
	ui.Printf("Saved: %s\n", filename)

	elapsed := time.Since(start).Seconds()
	ui.Printf("Completed in %.0fs\n", elapsed)
	return &RunResult{Videos: []string{filename}}, nil
}
