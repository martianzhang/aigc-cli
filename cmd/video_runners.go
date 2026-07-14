package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/service"
	"github.com/martianzhang/apimart-cli/internal/types"
)

// runAPIMartVideo handles video generation via APIMart async task API.
func runAPIMartVideo(req *types.VideoGenerateRequest) error {
	// Resolve local image files in image_urls
	if len(req.ImageURLs) > 0 {
		c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
		resolved, err := c.ResolveLocalImages(req.ImageURLs)
		if err != nil {
			return fmt.Errorf("failed to resolve image-urls: %w", err)
		}
		req.ImageURLs = resolved
	}
	// Resolve local image files in image_with_roles
	for i := range req.ImageWithRoles {
		c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
		resolved, err := c.ResolveLocalImages([]string{req.ImageWithRoles[i].URL})
		if err != nil {
			return fmt.Errorf("failed to resolve image-with-role: %w", err)
		}
		req.ImageWithRoles[i].URL = resolved[0]
	}

	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
	applyTimeout(c, "video", client.VideoTimeout)
	resp, err := c.VideoSubmit(req)
	if err != nil {
		return fmt.Errorf("submission failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return fmt.Errorf("submission returned no tasks")
	}

	task := resp.Data[0]
	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Response code: %d\n", resp.Code)
	fmt.Printf("Task ID: %s\n", task.TaskID)
	fmt.Printf("Status: %s\n\n", task.Status)

	fmt.Println("Polling for completion...")
	taskData, err := c.PollTask(task.TaskID)
	if err != nil {
		return fmt.Errorf("polling failed: %w", err)
	}

	if shared.Verbose {
		prettyResult, _ := json.MarshalIndent(taskData, "", "  ")
		fmt.Printf("\nTask result:\n%s\n", string(prettyResult))
	}

	fmt.Println()
	savePromptFile(taskData.ID, req.Prompt)
	if taskData.Result != nil && len(taskData.Result.Videos) > 0 {
		if _, err := downloadVideos(taskData.Result.Videos, taskData.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
		}
	}

	fmt.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
		taskData.ActualTime, taskData.Cost, taskData.CreditsCost)
	return nil
}

// runYunwuVideo handles video generation via yunwu.ai's unified API (submit -> poll -> download).
// Uses POST /v1/video/create for submission and GET /v1/video/query?id= for polling.
func runYunwuVideo(req *types.VideoGenerateRequest) error {
	// Resolve local images before submission
	if len(req.ImageURLs) > 0 {
		c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
		resolved, err := c.ResolveLocalImages(req.ImageURLs)
		if err != nil {
			return fmt.Errorf("failed to resolve image-urls: %w", err)
		}
		req.ImageURLs = resolved
	}
	for i := range req.ImageWithRoles {
		c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
		resolved, err := c.ResolveLocalImages([]string{req.ImageWithRoles[i].URL})
		if err != nil {
			return fmt.Errorf("failed to resolve image-with-role: %w", err)
		}
		req.ImageWithRoles[i].URL = resolved[0]
	}

	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
	applyTimeout(c, "video", client.VideoTimeout)

	// Step 1: Submit
	createResp, err := c.YunwuVideoSubmit(req)
	if err != nil {
		return fmt.Errorf("yunwu video submission failed: %w", err)
	}

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
			return fmt.Errorf("yunwu video polling timed out after %v", yunwuMaxWait)
		}

		queryResp, err := c.YunwuVideoQuery(taskID)
		if err != nil {
			return fmt.Errorf("polling failed: %w", err)
		}

		switch queryResp.Status {
		case "completed", "succeeded", "success":
			videoURL = queryResp.VideoURL
			if videoURL == "" {
				return fmt.Errorf("yunwu video completed but no video_url returned")
			}
		case "failed", "failure":
			return fmt.Errorf("yunwu video generation failed: status=%s", queryResp.Status)
		case "cancelled", "expired":
			return fmt.Errorf("yunwu video generation %s", queryResp.Status)
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
		return fmt.Errorf("failed to download video: %w", err)
	}
	fmt.Printf("Saved: %s\n", filename)

	elapsed := time.Since(start).Seconds()
	fmt.Printf("Completed in %.0fs\n", elapsed)
	return nil
}

// runVideoRemix handles the VEO3 remix (video extension) flow.
func runVideoRemix(cmd *cobra.Command) error {
	if vidTaskID == "" {
		return fmt.Errorf("--task-id is required in remix mode (the original video task ID)")
	}

	// Build remix request
	prompt, err := resolveVideoPrompt()
	if err != nil {
		return err
	}
	req := &types.VideoRemixRequest{
		Model:      shared.Model,
		Prompt:     prompt,
		Resolution: vidResolution,
	}
	if vidSize != "" {
		req.AspectRatio = vidSize // --size maps to aspect_ratio in remix
	}
	if cmd.Flags().Changed("raw") {
		v := vidRaw
		req.Raw = &v
	}

	if req.Model == "" {
		return fmt.Errorf("--model is required in remix mode (must match the original video's model)")
	}
	if req.Prompt == "" {
		return fmt.Errorf("--prompt is required in remix mode")
	}

	if vidDryRun {
		curl := buildVideoRemixCurl(req)
		fmt.Println(curl)
		return nil
	}

	if shared.Verbose {
		prettyReq, _ := json.MarshalIndent(req, "", "  ")
		fmt.Printf("Request:\n%s\n\n", string(prettyReq))
	}

	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
	applyTimeout(c, "video", client.VideoTimeout)
	resp, err := c.VideoRemixSubmit(vidTaskID, req)
	if err != nil {
		return fmt.Errorf("remix submission failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return fmt.Errorf("remix returned no tasks")
	}

	task := resp.Data[0]
	fmt.Printf("Response code: %d\n", resp.Code)
	fmt.Printf("Task ID: %s\n", task.TaskID)
	fmt.Printf("Status: %s\n\n", task.Status)

	fmt.Println("Polling for completion...")
	taskData, err := c.PollTask(task.TaskID)
	if err != nil {
		return fmt.Errorf("polling failed: %w", err)
	}

	if shared.Verbose {
		prettyResult, _ := json.MarshalIndent(taskData, "", "  ")
		fmt.Printf("\nTask result:\n%s\n", string(prettyResult))
	}

	fmt.Println()
	savePromptFile(taskData.ID, req.Prompt)
	if taskData.Result != nil && len(taskData.Result.Videos) > 0 {
		if _, err := downloadVideos(taskData.Result.Videos, taskData.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
		}
	}

	fmt.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
		taskData.ActualTime, taskData.Cost, taskData.CreditsCost)
	return nil
}

// runOpenRouterVideo handles video generation via OpenRouter's dedicated video API.
func runOpenRouterVideo(req *types.VideoGenerateRequest) error {
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

	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
	applyTimeout(c, "video", client.VideoTimeout)

	// Step 1: Submit
	submitResp, err := c.OpenRouterVideoSubmit(orReq)
	if err != nil {
		return fmt.Errorf("OpenRouter video submission failed: %w", err)
	}

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
		return fmt.Errorf("video polling failed: %w", err)
	}

	elapsed := time.Since(pollStart).Seconds()
	fmt.Printf("Completed in %.0fs\n\n", elapsed)

	if shared.Verbose {
		prettyResult, _ := json.MarshalIndent(pollResp, "", "  ")
		fmt.Printf("Video result:\n%s\n\n", string(prettyResult))
	}

	// Step 3: Download
	if len(pollResp.UnsignedURLs) == 0 {
		return fmt.Errorf("video job completed but no download URLs returned")
	}

	for i, u := range pollResp.UnsignedURLs {
		ext := extractExt(u)
		filename := filepath.Join(shared.OutputDir, fmt.Sprintf("video_%s_%d%s", submitResp.ID, i, ext))
		fmt.Printf("Downloading video %d/%d...\n", i+1, len(pollResp.UnsignedURLs))
		if err := service.SaveResource(u, filename); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to download video %d: %v\n", i, err)
			continue
		}
		fmt.Printf("Saved: %s\n", filename)
	}

	if pollResp.Usage != nil {
		fmt.Printf("Tokens: %d in / %d out", pollResp.Usage.InputTokens, pollResp.Usage.OutputTokens)
		if pollResp.Usage.TotalCost > 0 {
			fmt.Printf(" | Cost: $%.5f", pollResp.Usage.TotalCost)
		}
		fmt.Println()
	}

	return nil
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

	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
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

	for i, u := range statusResp.UnsignedURLs {
		ext := extractExt(u)
		filename := filepath.Join(shared.OutputDir, fmt.Sprintf("video_%s_%d%s", info.JobID, i, ext))
		fmt.Printf("Downloading video %d/%d...\n", i+1, len(statusResp.UnsignedURLs))
		if err := service.SaveResource(u, filename); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to download video %d: %v\n", i, err)
			continue
		}
		fmt.Printf("Saved: %s\n", filename)
	}

	if statusResp.Usage != nil {
		fmt.Printf("Tokens: %d in / %d out", statusResp.Usage.InputTokens, statusResp.Usage.OutputTokens)
		if statusResp.Usage.TotalCost > 0 {
			fmt.Printf(" | Cost: $%.5f", statusResp.Usage.TotalCost)
		}
		fmt.Println()
	}

	return nil
}
