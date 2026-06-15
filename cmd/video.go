package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/config"
	"github.com/martianzhang/apimart-cli/internal/types"
)

// video flag variables
var (
	vidModel           string
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
)

// videoCmd represents the `apimart-cli video` command.
var videoCmd = &cobra.Command{
	Use:   "video",
	Short: "Generate videos via the APIMart API",
	Long: `Generate videos using APIMart video models (doubao-seedance-2.0).

Supports text-to-video, image-to-video, first/last frame video,
reference video, and audio-enabled video.

Examples:
  apimart-cli video --prompt "A kitten yawning at the camera"
  apimart-cli video --prompt "City nightscape" --resolution 720p --duration 8
  apimart-cli video --prompt "..." --image-url ./cat.jpg
  apimart-cli video --prompt "Transition day to night" --first-frame day.jpg --last-frame night.jpg
  apimart-cli video --json request.json`,
	RunE: runVideo,
}

func runVideo(cmd *cobra.Command, args []string) error {
	req, err := buildVideoRequest(cmd)
	if err != nil {
		return err
	}

	// Merge config defaults
	if cfg, err := config.LoadDefaults(cfgFile); err == nil && cfg != nil && cfg.Defaults != nil {
		cfg.Defaults.Video.MergeIntoVideo(req)
	}

	// Apply hardcoded defaults
	if req.Model == "" {
		req.Model = "doubao-seedance-2.0"
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

	prettyReq, _ := json.MarshalIndent(req, "", "  ")
	fmt.Printf("Request:\n%s\n\n", string(prettyReq))

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

	prettyResult, _ := json.MarshalIndent(taskData, "", "  ")
	fmt.Printf("\nTask result:\n%s\n", string(prettyResult))

	if taskData.Result != nil && len(taskData.Result.Videos) > 0 {
		return downloadVideos(taskData.Result.Videos)
	}

	return nil
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

	// Resolve prompt (defaults to stdin)
	prompt := vidPrompt
	if prompt == "" {
		prompt = "-"
	}
	if prompt == "-" || isFile(prompt) {
		data, err := readInput(prompt)
		if err != nil {
			return nil, fmt.Errorf("failed to read prompt: %w", err)
		}
		prompt = string(data)
	}

	req := &types.VideoGenerateRequest{
		Model:      vidModel,
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
		base = "https://api.apimart.ai"
	}
	base = strings.TrimRight(base, "/")
	url := base + "/v1/videos/generations"

	cmd := fmt.Sprintf("curl -X POST %s \\\n", url)
	cmd += fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", apiKey)
	cmd += "  -H \"Content-Type: application/json\" \\\n"
	cmd += fmt.Sprintf("  -d '%s'", string(body))
	return cmd
}

// downloadVideos downloads all generated videos.
func downloadVideos(videos []types.VideoResult) error {
	for i, vid := range videos {
		for j, url := range vid.URL {
			resp, err := httpGet(url)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download video %d-%d: %v\n", i, j, err)
				continue
			}
			ext := extractExt(url)
			filename := filepath.Join(outputDir, fmt.Sprintf("apimart_video_%d_%d%s", i, j, ext))
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
	ext := filepath.Ext(rawURL)
	if ext == "" {
		return ".mp4"
	}
	return ext
}

func init() {
	f := videoCmd.Flags()
	f.StringVar(&vidModel, "model", "", `Model name (default "doubao-seedance-2.0")`)
	f.StringVar(&vidPrompt, "prompt", "", "Video content description")
	f.IntVar(&vidDuration, "duration", 0, "Video duration in seconds (4-15)")
	f.StringVar(&vidSize, "size", "", `Aspect ratio: 16:9, 9:16, 1:1, 4:3, 3:4, 21:9, adaptive`)
	f.StringVar(&vidResolution, "resolution", "", "Resolution: 480p, 720p, 1080p")
	f.IntVar(&vidSeed, "seed", 0, "Random seed for reproducibility")
	f.BoolVar(&vidGenerateAudio, "generate-audio", false, "Generate AI audio for the video")
	f.BoolVar(&vidReturnLastFrame, "return-last-frame", false, "Return the last frame image URL for continuation")
	f.StringArrayVar(&vidImageURLs, "image-url", nil, "Reference image URL (repeatable)")
	f.StringVar(&vidFirstFrame, "first-frame", "", "First frame image URL or local path")
	f.StringVar(&vidLastFrame, "last-frame", "", "Last frame image URL or local path")
	f.StringArrayVar(&vidVideoURLs, "video-url", nil, "Reference video URL (repeatable)")
	f.StringArrayVar(&vidAudioURLs, "audio-url", nil, "Reference audio URL (repeatable)")
	f.StringArrayVar(&vidTools, "tool", nil, "Tool type (e.g. web_search, repeatable)")
	f.BoolVar(&vidDryRun, "dry-run", false, "Print request parameters without calling API")

	rootCmd.AddCommand(videoCmd)
}
