package cmd

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// imageToDataURI 读取本地图片文件并转为 base64 data URI（如 data:image/jpeg;base64,...）。
// agnes 没有上传端点，图生视频需以 data URI 形式内嵌图片。
func imageToDataURI(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read image %q: %w", path, err)
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	switch ext {
	case "jpg", "jpeg":
		ext = "jpeg"
	case "png":
		ext = "png"
	case "webp":
		ext = "webp"
	case "gif":
		ext = "gif"
	case "bmp":
		ext = "bmp"
	default:
		ext = "png"
	}
	return fmt.Sprintf("data:image/%s;base64,%s", ext, base64.StdEncoding.EncodeToString(data)), nil
}

// runAgnesVideo handles video generation via agnes.ai's async task API.
// Uses POST /v1/videos for submission and GET /agnesapi?video_id= for polling.
func runAgnesVideo(req *types.VideoGenerateRequest) ([]string, error) {
	// agnes 没有上传端点，本地图片需转为 base64 data URI 内嵌。
	for i, u := range req.ImageURLs {
		if isFile(u) {
			uri, err := imageToDataURI(u)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve image-url %q: %w", u, err)
			}
			req.ImageURLs[i] = uri
		}
	}
	for i := range req.ImageWithRoles {
		if isFile(req.ImageWithRoles[i].URL) {
			uri, err := imageToDataURI(req.ImageWithRoles[i].URL)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve image-with-role %q: %w", req.ImageWithRoles[i].URL, err)
			}
			req.ImageWithRoles[i].URL = uri
		}
	}

	c := newCmdClient("video")
	applyTimeout(c, "video", client.VideoTimeout)

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
	filename, err := service.DownloadFile(videoURL, shared.OutputDir, fmt.Sprintf("video_agnes_%s", videoID))
	if err != nil {
		return nil, fmt.Errorf("failed to download video: %w", err)
	}
	fmt.Printf("Saved: %s\n", filename)

	elapsed := time.Since(start).Seconds()
	fmt.Printf("Completed in %.0fs\n", elapsed)
	return []string{filename}, nil
}
