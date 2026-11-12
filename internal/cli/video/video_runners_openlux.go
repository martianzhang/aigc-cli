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

// runOpenLuxVideo handles video generation via api.openlux.ai's unified API (submit -> poll -> download).
// Uses POST /v1/video/create for submission and GET /v1/video/query?id= for polling.
// Local images are uploaded by videoPlan.applyUploads before dispatch.
func runOpenLuxVideo(ctx context.Context, c *client.Client, p *provider.EffectiveProvider, req *types.VideoGenerateRequest) (*RunResult, error) {
	// Step 1: Submit
	createResp, err := c.OpenLuxVideoSubmit(req)
	if err != nil {
		return nil, fmt.Errorf("openlux video submission failed: %w", err)
	}

	ui.Printf("Provider: %s\n", p.ProviderType)
	ui.Printf("Model: %s\n", req.Model)
	ui.Printf("Task ID: %s\n", createResp.ID)
	ui.Printf("Status: %s\n\n", createResp.Status)

	// Step 2: Poll
	ui.Println("Polling for completion...")
	taskID := createResp.ID
	openluxMaxWait := clientfactory.TaskWait(types.MediaVideo)
	start := time.Now()
	var videoURL string
	for {
		if time.Since(start) > openluxMaxWait {
			return nil, fmt.Errorf("openlux video polling timed out after %v", openluxMaxWait)
		}

		queryResp, err := c.OpenLuxVideoQuery(taskID)
		if err != nil {
			return nil, fmt.Errorf("polling failed: %w", err)
		}

		switch queryResp.Status {
		case "completed", "succeeded", "success":
			videoURL = queryResp.VideoURL
			if videoURL == "" {
				return nil, fmt.Errorf("openlux video completed but no video_url returned")
			}
		case "failed", "failure":
			return nil, fmt.Errorf("openlux video generation failed: status=%s", queryResp.Status)
		case "cancelled", "expired":
			return nil, fmt.Errorf("openlux video generation %s", queryResp.Status)
		default:
			// pending / running / in_progress / queued -- keep waiting
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
	service.SavePromptBestEffort(options.Shared.SavePrompt, options.Shared.OutputDir, "video", taskID, req.Prompt)
	filename, err := service.DownloadFile(videoURL, options.Shared.OutputDir, fmt.Sprintf("video_openlux_%s", taskID))
	if err != nil {
		return nil, fmt.Errorf("failed to download video: %w", err)
	}
	ui.Printf("Saved: %s\n", filename)

	elapsed := time.Since(start).Seconds()
	ui.Printf("Completed in %.0fs\n", elapsed)
	return &RunResult{Videos: []string{filename}}, nil
}
