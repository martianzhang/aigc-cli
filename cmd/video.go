package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/config"
	"github.com/martianzhang/apimart-cli/internal/types"
)

// video flag variables
var (
	vidPrompt          string
	vidDuration        int
	vidSize            string
	vidResolution      string
	vidSeed            int
	vidGenerateAudio   bool
	vidReturnLastFrame bool
	vidImageURLs       []string
	vidFirstFrame      string
	vidLastFrame       string
	vidVideoURLs       []string
	vidAudioURLs       []string
	vidDryRun          bool
	vidTools           []string
	vidRemix           bool
	vidRaw             bool
	vidTaskID          string
	vidJobID           string // OpenRouter video job ID for resume
)

// openRouterJobInfo is saved to disk so the user can resume a timed-out video job.
type openRouterJobInfo struct {
	JobID      string `json:"job_id"`
	PollingURL string `json:"polling_url"`
	Model      string `json:"model"`
	Prompt     string `json:"prompt"`
	CreatedAt  int64  `json:"created_at"`
}

func jobFilePath(jobID string) string {
	return filepath.Join(outputDir, fmt.Sprintf("video_job_%s.json", jobID))
}

func saveJobInfo(info *openRouterJobInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(jobFilePath(info.JobID), data, 0644)
}

func loadJobInfo(jobID string) (*openRouterJobInfo, error) {
	data, err := os.ReadFile(jobFilePath(jobID))
	if err != nil {
		return nil, fmt.Errorf("job file %s not found (was the job submitted with this output directory?): %w", jobFilePath(jobID), err)
	}
	var info openRouterJobInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("failed to parse job file: %w", err)
	}
	return &info, nil
}

// videoCmd represents the `apimart-cli video` command.
var videoCmd = &cobra.Command{
	Use:          "video",
	Short:        "Generate videos via the APIMart API",
	SilenceUsage: true,
	Long: `Generate videos using APIMart video models (doubao-seedance-2.0).

Supports text-to-video, image-to-video, first/last frame video,
reference video, audio-enabled video, and VEO3 video remix.

Remix mode (--remix):
  VEO3 Remix extends a generated video from 8s to 15s.
  Requires --remix + --task-id + --prompt + --model.
  The model must match the original video's model.

Examples:
  apimart-cli video --prompt "A kitten yawning at the camera"
  apimart-cli video --prompt "City nightscape" --resolution 720p --duration 8
  apimart-cli video --prompt "..." --image-url ./cat.jpg
  apimart-cli video --prompt "Transition day to night" --first-frame day.jpg --last-frame night.jpg
  apimart-cli video --json request.json
  apimart-cli video --remix --task-id task_xxx --model veo3.1-fast --prompt "continue running"
  apimart-cli video --remix --task-id task_xxx --model veo3.1-fast --prompt "keep going" --raw --resolution 1080p`,
	RunE: runVideo,
}

func runVideo(cmd *cobra.Command, args []string) error {
	if vidRemix {
		return runVideoRemix(cmd)
	}

	// Resume an existing OpenRouter video job (--job-id)
	if vidJobID != "" {
		return runOpenRouterVideoResume(vidJobID)
	}

	req, err := buildVideoRequest(cmd)
	if err != nil {
		return err
	}

	// Merge config defaults
	if cfg, err := config.LoadDefaults(cfgFile); err == nil && cfg != nil && cfg.Defaults != nil {
		cfg.Defaults.Video.MergeIntoVideo(req)
	}

	// Apply defaults for remaining empty fields
	if req.Model == "" {
		return fmt.Errorf("model is required: set via --model flag or defaults.video.model in config.yaml")
	}
	if req.Size == "" {
		req.Size = "16:9"
	}
	if req.Resolution == "" {
		req.Resolution = "480p"
	}

	if vidDryRun {
		curl := buildVideoCurl(req)
		fmt.Println(curl)
		return nil
	}

	if verbose {
		prettyReq, _ := json.MarshalIndent(req, "", "  ")
		fmt.Printf("Request:\n%s\n\n", string(prettyReq))
	}

	// Strategy table: first match wins, last entry is the default.
	for _, s := range videoStrategies {
		if s.match(req) {
			return s.run(req)
		}
	}
	return nil
}

// videoStrategy defines a dispatch rule for video generation.
type videoStrategy struct {
	match func(*types.VideoGenerateRequest) bool
	run   func(*types.VideoGenerateRequest) error
}

// videoStrategies is the ordered dispatch table for video generation.
// First match wins. Add a new entry here when adding a new provider.
var videoStrategies = []videoStrategy{
	{
		// OpenRouter: dedicated video API (submit → poll → download)
		match: func(req *types.VideoGenerateRequest) bool {
			return isOpenRouterProvider()
		},
		run: runOpenRouterVideo,
	},
	{
		// Default: APIMart async task-based generation
		match: func(req *types.VideoGenerateRequest) bool { return true },
		run:   runAPIMartVideo,
	},
}

// runAPIMartVideo handles video generation via APIMart async task API.
func runAPIMartVideo(req *types.VideoGenerateRequest) error {
	// Resolve local image files in image_urls
	if len(req.ImageURLs) > 0 {
		c := client.New(apiKey, apiBase, httpProxy)
		resolved, err := c.ResolveLocalImages(req.ImageURLs)
		if err != nil {
			return fmt.Errorf("failed to resolve image-urls: %w", err)
		}
		req.ImageURLs = resolved
	}
	// Resolve local image files in image_with_roles
	for i := range req.ImageWithRoles {
		c := client.New(apiKey, apiBase, httpProxy)
		resolved, err := c.ResolveLocalImages([]string{req.ImageWithRoles[i].URL})
		if err != nil {
			return fmt.Errorf("failed to resolve image-with-role: %w", err)
		}
		req.ImageWithRoles[i].URL = resolved[0]
	}

	c := client.New(apiKey, apiBase, httpProxy)
	resp, err := c.VideoSubmit(req)
	if err != nil {
		return fmt.Errorf("submission failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return fmt.Errorf("submission returned no tasks")
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

	if verbose {
		prettyResult, _ := json.MarshalIndent(taskData, "", "  ")
		fmt.Printf("\nTask result:\n%s\n", string(prettyResult))
	}

	fmt.Println()
	savePromptFile(taskData.ID, req.Prompt)
	if taskData.Result != nil && len(taskData.Result.Videos) > 0 {
		if err := downloadVideos(taskData.Result.Videos, taskData.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
		}
	}

	fmt.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
		taskData.ActualTime, taskData.Cost, taskData.CreditsCost)
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
		Model:      model,
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

	if verbose {
		prettyReq, _ := json.MarshalIndent(req, "", "  ")
		fmt.Printf("Request:\n%s\n\n", string(prettyReq))
	}

	c := client.New(apiKey, apiBase, httpProxy)
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

	if verbose {
		prettyResult, _ := json.MarshalIndent(taskData, "", "  ")
		fmt.Printf("\nTask result:\n%s\n", string(prettyResult))
	}

	fmt.Println()
	savePromptFile(taskData.ID, req.Prompt)
	if taskData.Result != nil && len(taskData.Result.Videos) > 0 {
		if err := downloadVideos(taskData.Result.Videos, taskData.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
		}
	}

	fmt.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
		taskData.ActualTime, taskData.Cost, taskData.CreditsCost)
	return nil
}

// resolveVideoPrompt resolves the video prompt (shared by normal and remix modes).
func resolveVideoPrompt() (string, error) {
	prompt := vidPrompt
	if prompt == "" {
		prompt = "-"
	}
	if prompt == "-" || isFile(prompt) {
		data, err := readInput(prompt)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt: %w", err)
		}
		return string(data), nil
	}
	return prompt, nil
}

func buildVideoRequest(cmd *cobra.Command) (*types.VideoGenerateRequest, error) {
	if jsonInput != "" {
		data, err := readInput(jsonInput)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON input: %w", err)
		}
		req := &types.VideoGenerateRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		return req, nil
	}

	prompt, err := resolveVideoPrompt()
	if err != nil {
		return nil, err
	}

	req := &types.VideoGenerateRequest{
		Model:      model,
		Prompt:     prompt,
		Size:       vidSize,
		Resolution: vidResolution,
		ImageURLs:  vidImageURLs,
		VideoURLs:  vidVideoURLs,
		AudioURLs:  vidAudioURLs,
	}

	setIntFlag(cmd, "duration", &req.Duration, vidDuration)
	setIntFlag(cmd, "seed", &req.Seed, vidSeed)
	setBoolFlag(cmd, "generate-audio", &req.GenerateAudio, vidGenerateAudio)
	setBoolFlag(cmd, "return-last-frame", &req.ReturnLastFrame, vidReturnLastFrame)

	// --first-frame / --last-frame → image_with_roles
	if cmd.Flags().Changed("first-frame") || cmd.Flags().Changed("last-frame") {
		var roles []types.ImageWithRole
		if cmd.Flags().Changed("first-frame") {
			roles = append(roles, types.ImageWithRole{URL: vidFirstFrame, Role: "first_frame"})
		}
		if cmd.Flags().Changed("last-frame") {
			roles = append(roles, types.ImageWithRole{URL: vidLastFrame, Role: "last_frame"})
		}
		req.ImageWithRoles = roles
	}

	// --tool
	for _, t := range vidTools {
		req.Tools = append(req.Tools, types.VideoTool{Type: t})
	}

	return req, nil
}

// setIntFlag sets a *int field from a cobra flag if changed.
func setIntFlag(cmd *cobra.Command, name string, target **int, val int) {
	if cmd.Flags().Changed(name) {
		v := val
		*target = &v
	}
}

// setBoolFlag sets a *bool field from a cobra flag if changed.
func setBoolFlag(cmd *cobra.Command, name string, target **bool, val bool) {
	if cmd.Flags().Changed(name) {
		v := val
		*target = &v
	}
}

func buildVideoCurl(req *types.VideoGenerateRequest) string {
	body, _ := json.Marshal(req)
	base := apiBase
	if base == "" {
		base = "https://api.apimart.ai/v1" // matches client.defaultBaseURL
	}
	base = strings.TrimRight(base, "/")
	url := base + "/videos/generations"

	cmd := fmt.Sprintf("curl -X POST %s \\\n", url)
	cmd += fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", apiKey)
	cmd += "  -H \"Content-Type: application/json\" \\\n"
	cmd += fmt.Sprintf("  -d '%s'", string(body))
	return cmd
}

func buildVideoRemixCurl(req *types.VideoRemixRequest) string {
	body, _ := json.Marshal(req)
	base := apiBase
	if base == "" {
		base = "https://api.apimart.ai/v1"
	}
	base = strings.TrimRight(base, "/")
	url := fmt.Sprintf("%s/videos/%s/remix", base, vidTaskID)

	cmd := fmt.Sprintf("curl -X POST %s \\\n", url)
	cmd += fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", apiKey)
	cmd += "  -H \"Content-Type: application/json\" \\\n"
	cmd += fmt.Sprintf("  -d '%s'", string(body))
	return cmd
}

// downloadVideos downloads all generated videos.
func downloadVideos(videos []types.VideoResult, taskID string) error {
	for i, vid := range videos {
		for j, url := range vid.URL {
			resp, err := httpGet(url)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download video %d-%d: %v\n", i, j, err)
				continue
			}
			ext := extractExt(url)
			filename := filepath.Join(outputDir, fmt.Sprintf("video_%s_%d_%d%s", taskID, i, j, ext))
			if err := os.WriteFile(filename, resp, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", filename, err)
				continue
			}
			fmt.Printf("Saved: %s\n", filename)
		}
	}
	return nil
}

// extractExt returns the file extension from a URL, defaulting to .mp4.
func extractExt(rawURL string) string {
	// Strip query params
	if idx := strings.Index(rawURL, "?"); idx != -1 {
		rawURL = rawURL[:idx]
	}
	ext := filepath.Ext(rawURL)
	if ext == "" {
		return ".mp4"
	}
	return ext
}

// ---------------------------------------------------------------------------
// OpenRouter Video — submit → poll → download
// ---------------------------------------------------------------------------

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

	// Map image_urls → frame_images
	for _, u := range req.ImageURLs {
		frame := types.OpenRouterFrameImage{}
		frame.Type = "image_url"
		frame.ImageURL.URL = u
		frame.FrameType = "first_frame"
		orReq.FrameImages = append(orReq.FrameImages, frame)
	}
	// Map image_with_roles → frame_images
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

	if verbose {
		prettyReq, _ := json.MarshalIndent(orReq, "", "  ")
		fmt.Printf("OpenRouter Video Request:\n%s\n\n", string(prettyReq))
	}

	c := client.New(apiKey, apiBase, httpProxy)

	// Step 1: Submit
	submitResp, err := c.OpenRouterVideoSubmit(orReq)
	if err != nil {
		return fmt.Errorf("OpenRouter video submission failed: %w", err)
	}

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
	fmt.Println("Polling for completion (this may take 30s–a few minutes)...")
	pollStart := time.Now()
	pollResp, err := c.OpenRouterVideoPollUntilComplete(submitResp.PollingURL, 30*time.Second, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("video polling failed: %w", err)
	}

	elapsed := time.Since(pollStart).Seconds()
	fmt.Printf("Completed in %.0fs\n\n", elapsed)

	if verbose {
		prettyResult, _ := json.MarshalIndent(pollResp, "", "  ")
		fmt.Printf("Video result:\n%s\n\n", string(prettyResult))
	}

	// Step 3: Download
	if len(pollResp.UnsignedURLs) == 0 {
		return fmt.Errorf("video job completed but no download URLs returned")
	}

	for i, u := range pollResp.UnsignedURLs {
		ext := extractExt(u)
		ts := time.Now().Unix()
		filename := filepath.Join(outputDir, fmt.Sprintf("video_%s_%d_%d%s", submitResp.ID, i, ts, ext))
		fmt.Printf("Downloading video %d/%d...\n", i+1, len(pollResp.UnsignedURLs))
		if err := c.OpenRouterVideoDownload(u, filename); err != nil {
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

	c := client.New(apiKey, apiBase, httpProxy)

	// Check current status
	statusResp, err := c.OpenRouterVideoGet(info.JobID)
	if err != nil {
		return fmt.Errorf("failed to query job %s: %w", info.JobID, err)
	}

	switch statusResp.Status {
	case "completed":
		// Already done — download directly
	case "failed", "cancelled", "expired":
		errMsg := statusResp.Error
		if errMsg == "" {
			errMsg = statusResp.Status
		}
		return fmt.Errorf("video job %s is %s: %s", info.JobID, statusResp.Status, errMsg)
	default:
		// pending / running — poll
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
		ts := time.Now().Unix()
		filename := filepath.Join(outputDir, fmt.Sprintf("video_%s_%d_%d%s", info.JobID, i, ts, ext))
		fmt.Printf("Downloading video %d/%d...\n", i+1, len(statusResp.UnsignedURLs))
		if err := c.OpenRouterVideoDownload(u, filename); err != nil {
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

func init() {
	f := videoCmd.Flags()
	f.StringVarP(&vidPrompt, "prompt", "p", "", "Video content description")
	f.IntVarP(&vidDuration, "duration", "d", 0, "Video duration in seconds (4-15)")
	f.StringVarP(&vidSize, "size", "s", "", `Aspect ratio: 16:9, 9:16, 1:1, 4:3, 3:4, 21:9, adaptive`)
	f.StringVarP(&vidResolution, "resolution", "r", "", "Resolution: 480p, 720p, 1080p (remix: 4k)")
	f.IntVar(&vidSeed, "seed", 0, "Random seed for reproducibility")
	f.BoolVarP(&vidGenerateAudio, "generate-audio", "a", false, "Generate AI audio for the video")
	f.BoolVar(&vidReturnLastFrame, "return-last-frame", false, "Return the last frame image URL for continuation")
	f.StringArrayVar(&vidImageURLs, "image-url", nil, "Reference image URL (repeatable)")
	f.StringVar(&vidFirstFrame, "first-frame", "", "First frame image URL or local path")
	f.StringVar(&vidLastFrame, "last-frame", "", "Last frame image URL or local path")
	f.StringArrayVar(&vidVideoURLs, "video-url", nil, "Reference video URL (repeatable)")
	f.StringArrayVar(&vidAudioURLs, "audio-url", nil, "Reference audio URL (repeatable)")
	f.StringArrayVar(&vidTools, "tool", nil, "Tool type (e.g. web_search, repeatable)")
	f.BoolVar(&vidRemix, "remix", false, "VEO3 Remix mode: extend video from 8s to 15s (requires --task-id)")
	f.BoolVar(&vidRaw, "raw", false, "Remix: return only the extended portion (VEO3 remix only)")
	f.StringVar(&vidTaskID, "task-id", "", "Original video task ID for remix (required with --remix)")
	f.BoolVar(&vidDryRun, "dry-run", false, "Print request parameters without calling API")
	f.StringVar(&jsonInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	f.StringVar(&vidJobID, "job-id", "", "Resume an OpenRouter video job by ID (loads saved job info and downloads the result)")

	rootCmd.AddCommand(videoCmd)
}
