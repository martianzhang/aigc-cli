package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runOpenRouterVideo handles video generation via OpenRouter's dedicated video API.
func runOpenRouterVideo(req *types.VideoGenerateRequest) ([]string, error) {
	// Build OpenRouter video request
	orReq := &types.OpenRouterVideoRequest{
		Model:         req.Model,
		Prompt:        req.Prompt,
		AspectRatio:   req.Size,
		Resolution:    req.Resolution,
		Duration:      req.Duration,
		Seed:          req.Seed,
		GenerateAudio: req.GenerateAudio,
	}

	// Map image_urls -> frame_images
	for _, u := range req.ImageURLs {
		frame := types.OpenRouterFrameImage{}
		frame.Type = "image_url"
		frame.ImageURL.URL = u
		frame.FrameType = "first_frame"
		orReq.FrameImages = append(orReq.FrameImages, frame)
	}
	// Map image_with_roles -> frame_images
	for _, r := range req.ImageWithRoles {
		frame := types.OpenRouterFrameImage{}
		frame.Type = "image_url"
		frame.ImageURL.URL = r.URL
		switch r.Role {
		case "first_frame":
			frame.FrameType = "first_frame"
		case "last_frame":
			frame.FrameType = "last_frame"
		}
		orReq.FrameImages = append(orReq.FrameImages, frame)
	}

	if shared.Verbose {
		prettyReq, _ := json.MarshalIndent(orReq, "", "  ")
		fmt.Printf("OpenRouter Video Request:\n%s\n\n", string(prettyReq))
	}

	c := newCmdClient("video")
	applyTimeout(c, "video", client.VideoTimeout)

	// Step 1: Submit
	submitResp, err := c.OpenRouterVideoSubmit(orReq)
	if err != nil {
		return nil, fmt.Errorf("OpenRouter video submission failed: %w", err)
	}

	fmt.Printf("Provider: %s\n", shared.ResolveProvider("video").ProviderType)
	fmt.Printf("Model: %s\n", orReq.Model)
	fmt.Printf("Video job submitted.\n")
	fmt.Printf("Job ID: %s\n", submitResp.ID)
	fmt.Printf("Status: %s\n\n", submitResp.Status)

	// Save job info for later resume
	jobInfo := &openRouterJobInfo{
		JobID:      submitResp.ID,
		PollingURL: submitResp.PollingURL,
		Model:      orReq.Model,
		Prompt:     orReq.Prompt,
		CreatedAt:  time.Now().Unix(),
	}
	if err := saveJobInfo(jobInfo); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save job info: %v\n", err)
	} else {
		fmt.Printf("Job info saved. Resume later with: --job-id %s\n", submitResp.ID)
	}

	// Step 2: Poll
	fmt.Println("Polling for completion (this may take 30s-a few minutes)...")
	pollStart := time.Now()
	pollResp, err := c.OpenRouterVideoPollUntilComplete(submitResp.PollingURL, 30*time.Second, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("video polling failed: %w", err)
	}

	elapsed := time.Since(pollStart).Seconds()
	fmt.Printf("Completed in %.0fs\n\n", elapsed)

	if shared.Verbose {
		prettyResult, _ := json.MarshalIndent(pollResp, "", "  ")
		fmt.Printf("Video result:\n%s\n\n", string(prettyResult))
	}

	// Step 3: Download
	if len(pollResp.UnsignedURLs) == 0 {
		return nil, fmt.Errorf("video job completed but no download URLs returned")
	}

	var saved []string
	for i, u := range pollResp.UnsignedURLs {
		ext := extractExt(u)
		filename := filepath.Join(shared.OutputDir, fmt.Sprintf("video_%s_%d%s", submitResp.ID, i, ext))
		fmt.Printf("Downloading video %d/%d...\n", i+1, len(pollResp.UnsignedURLs))
		if err := service.SaveResource(u, filename); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to download video %d: %v\n", i, err)
			continue
		}
		fmt.Printf("Saved: %s\n", filename)
		saved = append(saved, filename)
	}

	if pollResp.Usage != nil {
		fmt.Printf("Tokens: %d in / %d out", pollResp.Usage.InputTokens, pollResp.Usage.OutputTokens)
		if pollResp.Usage.TotalCost > 0 {
			fmt.Printf(" | Cost: $%.5f", pollResp.Usage.TotalCost)
		}
		fmt.Println()
	}

	return saved, nil
}

// runOpenRouterVideoResume resumes a previously-submitted OpenRouter video job.
// Loads saved job info, polls for completion (or uses cached result), and downloads the video.
func runOpenRouterVideoResume(jobID string) error {
	info, err := loadJobInfo(jobID)
	if err != nil {
		return err
	}

	fmt.Printf("Resuming video job: %s\n", info.JobID)
	fmt.Printf("Model: %s | Created: %s\n", info.Model, time.Unix(info.CreatedAt, 0).Format("2006-01-02 15:04:05"))
	fmt.Printf("Prompt: %s\n\n", info.Prompt)

	c := newCmdClient("video")
	applyTimeout(c, "video", client.VideoTimeout)

	// Check current status
	statusResp, err := c.OpenRouterVideoGet(info.JobID)
	if err != nil {
		return fmt.Errorf("failed to query job %s: %w", info.JobID, err)
	}

	switch statusResp.Status {
	case "completed":
		// Already done -- download directly
	case "failed", "cancelled", "expired":
		errMsg := statusResp.Error
		if errMsg == "" {
			errMsg = statusResp.Status
		}
		return fmt.Errorf("video job %s is %s: %s", info.JobID, statusResp.Status, errMsg)
	default:
		// pending / running -- poll
		fmt.Printf("Job status: %s. Polling for completion...\n", statusResp.Status)
		pollResp, err := c.OpenRouterVideoPollUntilComplete(info.PollingURL, 30*time.Second, 5*time.Minute)
		if err != nil {
			return fmt.Errorf("polling failed: %w", err)
		}
		statusResp = pollResp
	}

	// Download
	if len(statusResp.UnsignedURLs) == 0 {
		return fmt.Errorf("job completed but no download URLs returned")
	}

	var saved []string
	for i, u := range statusResp.UnsignedURLs {
		ext := extractExt(u)
		filename := filepath.Join(shared.OutputDir, fmt.Sprintf("video_%s_%d%s", info.JobID, i, ext))
		fmt.Printf("Downloading video %d/%d...\n", i+1, len(statusResp.UnsignedURLs))
		if err := service.SaveResource(u, filename); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to download video %d: %v\n", i, err)
			continue
		}
		fmt.Printf("Saved: %s\n", filename)
		saved = append(saved, filename)
	}
	if vidCropMargin != "" {
		if cropped, cerr := cropSavedVideos(saved); cerr != nil {
			fmt.Fprintf(os.Stderr, "Warning: video crop failed: %v\n", cerr)
		} else {
			saved = cropped
		}
	}

	if statusResp.Usage != nil {
		fmt.Printf("Tokens: %d in / %d out", statusResp.Usage.InputTokens, statusResp.Usage.OutputTokens)
		if statusResp.Usage.TotalCost > 0 {
			fmt.Printf(" | Cost: $%.5f", statusResp.Usage.TotalCost)
		}
		fmt.Println()
	}

	if vidPreview {
		for _, f := range saved {
			if e := service.PreviewFile(f); e != nil {
				fmt.Fprintf(os.Stderr, "Warning: preview failed: %v\n", e)
			}
		}
	}

	return nil
}
