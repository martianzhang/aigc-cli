package video

import (
	"fmt"
	"time"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runAgnesVideo handles video generation via agnes.ai's async task API.
// Uses POST /v1/videos for submission and GET /agnesapi?video_id= for polling.
// agnes has no upload endpoint, so buildVideoPlan embeds local files as data URIs.
func runAgnesVideo(req *types.VideoGenerateRequest) ([]string, error) {
	c := options.NewClient("video")
	options.ApplyTimeout(c, "video", client.VideoTimeout)

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

	fmt.Printf("Provider: %s\n", options.Shared.ResolveProvider("video").ProviderType)
	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Task ID: %s\n", createResp.TaskID)
	fmt.Printf("Status: %s\n\n", createResp.Status)

	// Step 2: Poll
	fmt.Println("Polling for completion...")
	const (
		agnesPollInterval = 15 * time.Second
		agnesMaxWait      = 10 * time.Minute
	)
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
			fmt.Printf("  Status: %s, Elapsed: %s\n", queryResp.Status, progress)
			time.Sleep(agnesPollInterval)
		}

		if videoURL != "" {
			break
		}
	}

	// Step 3: Download
	fmt.Println()
	fmt.Printf("Downloading video...\n")
	filename, err := service.DownloadFile(videoURL, options.Shared.OutputDir, fmt.Sprintf("video_agnes_%s", videoID))
	if err != nil {
		return nil, fmt.Errorf("failed to download video: %w", err)
	}
	fmt.Printf("Saved: %s\n", filename)

	elapsed := time.Since(start).Seconds()
	fmt.Printf("Completed in %.0fs\n", elapsed)
	return []string{filename}, nil
}
