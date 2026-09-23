package video

import (
	"fmt"
	"time"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runOpenLuxVideo handles video generation via api.openlux.ai's unified API (submit -> poll -> download).
// Uses POST /v1/video/create for submission and GET /v1/video/query?id= for polling.
// Local images are uploaded by videoPlan.applyUploads before dispatch.
func runOpenLuxVideo(req *types.VideoGenerateRequest) ([]string, error) {
	c := options.NewClient("video")
	options.ApplyTimeout(c, "video", client.VideoTimeout)

	// Step 1: Submit
	createResp, err := c.OpenLuxVideoSubmit(req)
	if err != nil {
		return nil, fmt.Errorf("openlux video submission failed: %w", err)
	}

	fmt.Printf("Provider: %s\n", options.Shared.ResolveProvider("video").ProviderType)
	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Task ID: %s\n", createResp.ID)
	fmt.Printf("Status: %s\n\n", createResp.Status)

	// Step 2: Poll
	fmt.Println("Polling for completion...")
	taskID := createResp.ID
	const (
		openluxPollInterval = 10 * time.Second
		openluxMaxWait      = 5 * time.Minute
	)
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
			fmt.Printf("  Status: %s, Elapsed: %s\n", queryResp.Status, progress)
			time.Sleep(openluxPollInterval)
		}

		if videoURL != "" {
			break
		}
	}

	// Step 3: Download
	fmt.Println()
	fmt.Printf("Downloading video...\n")
	filename, err := service.DownloadFile(videoURL, options.Shared.OutputDir, fmt.Sprintf("video_openlux_%s", taskID))
	if err != nil {
		return nil, fmt.Errorf("failed to download video: %w", err)
	}
	fmt.Printf("Saved: %s\n", filename)

	elapsed := time.Since(start).Seconds()
	fmt.Printf("Completed in %.0fs\n", elapsed)
	return []string{filename}, nil
}
