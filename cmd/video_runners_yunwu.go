package cmd

import (
	"fmt"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runYunwuVideo handles video generation via yunwu.ai's unified API (submit -> poll -> download).
// Uses POST /v1/video/create for submission and GET /v1/video/query?id= for polling.
func runYunwuVideo(req *types.VideoGenerateRequest) ([]string, error) {
	// Resolve local images before submission
	if len(req.ImageURLs) > 0 {
		c := newCmdClient("video")
		resolved, err := c.ResolveLocalImages(req.ImageURLs)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve image-urls: %w", err)
		}
		req.ImageURLs = resolved
	}
	for i := range req.ImageWithRoles {
		c := newCmdClient("video")
		resolved, err := c.ResolveLocalImages([]string{req.ImageWithRoles[i].URL})
		if err != nil {
			return nil, fmt.Errorf("failed to resolve image-with-role: %w", err)
		}
		req.ImageWithRoles[i].URL = resolved[0]
	}

	c := newCmdClient("video")
	applyTimeout(c, "video", client.VideoTimeout)

	// Step 1: Submit
	createResp, err := c.YunwuVideoSubmit(req)
	if err != nil {
		return nil, fmt.Errorf("yunwu video submission failed: %w", err)
	}

	fmt.Printf("Provider: %s\n", shared.ResolveProvider("video").ProviderType)
	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Task ID: %s\n", createResp.ID)
	fmt.Printf("Status: %s\n\n", createResp.Status)

	// Step 2: Poll
	fmt.Println("Polling for completion...")
	taskID := createResp.ID
	const (
		yunwuPollInterval = 10 * time.Second
		yunwuMaxWait      = 5 * time.Minute
	)
	start := time.Now()
	var videoURL string
	for {
		if time.Since(start) > yunwuMaxWait {
			return nil, fmt.Errorf("yunwu video polling timed out after %v", yunwuMaxWait)
		}

		queryResp, err := c.YunwuVideoQuery(taskID)
		if err != nil {
			return nil, fmt.Errorf("polling failed: %w", err)
		}

		switch queryResp.Status {
		case "completed", "succeeded", "success":
			videoURL = queryResp.VideoURL
			if videoURL == "" {
				return nil, fmt.Errorf("yunwu video completed but no video_url returned")
			}
		case "failed", "failure":
			return nil, fmt.Errorf("yunwu video generation failed: status=%s", queryResp.Status)
		case "cancelled", "expired":
			return nil, fmt.Errorf("yunwu video generation %s", queryResp.Status)
		default:
			// pending / running / in_progress / queued -- keep waiting
			progress := fmt.Sprintf("%.0fs", time.Since(start).Seconds())
			fmt.Printf("  Status: %s, Elapsed: %s\n", queryResp.Status, progress)
			time.Sleep(yunwuPollInterval)
		}

		if videoURL != "" {
			break
		}
	}

	// Step 3: Download
	fmt.Println()
	fmt.Printf("Downloading video...\n")
	filename, err := service.DownloadFile(videoURL, shared.OutputDir, fmt.Sprintf("video_yunwu_%s", taskID))
	if err != nil {
		return nil, fmt.Errorf("failed to download video: %w", err)
	}
	fmt.Printf("Saved: %s\n", filename)

	elapsed := time.Since(start).Seconds()
	fmt.Printf("Completed in %.0fs\n", elapsed)
	return []string{filename}, nil
}
