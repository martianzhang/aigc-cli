package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
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
	vidPreview         bool
	vidGIF             bool
	vidGIFWidth        int
	vidFFmpegFlags     string
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
	return filepath.Join(shared.OutputDir, fmt.Sprintf("video_job_%s.json", jobID))
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

// videoCmd represents the `aigc-cli video` command.
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
  aigc-cli video --prompt "A kitten yawning at the camera"
  aigc-cli video --prompt "City nightscape" --resolution 720p --duration 8
  aigc-cli video --prompt "..." --image-url ./cat.jpg
  aigc-cli video --prompt "Transition day to night" --first-frame day.jpg --last-frame night.jpg
  aigc-cli video --json request.json
  aigc-cli video --remix --task-id task_xxx --model veo3.1-fast --prompt "continue running"
  aigc-cli video --remix --task-id task_xxx --model veo3.1-fast --prompt "keep going" --raw --resolution 1080p
  aigc-cli video --prompt "A man doing push-ups" --gif            # generate then convert to GIF
  aigc-cli video --gif -i pushup.mp4                               # convert an existing local video`,
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

	// 本地视频转 GIF：--gif -i <本地文件>（无 prompt）时纯本地转换，不调生成 API。
	// -i 是 --image-url 的简写（对齐 image 命令）；仅当值是本地文件且未给 prompt 时走此分支。
	if vidGIF && vidPrompt == "" && len(vidImageURLs) > 0 {
		for _, u := range vidImageURLs {
			if isFile(u) {
				return convertLocalToGIF(u)
			}
		}
		return fmt.Errorf("--gif with --image-url/-i and no --prompt requires a local video file, got remote URL: %s", vidImageURLs[0])
	}

	// GIF 后处理需要 ffmpeg，提前校验避免浪费一次 API 调用。
	if vidGIF {
		if err := ensureGIFAvailable(); err != nil {
			return err
		}
	}

	req, err := buildVideoRequest(cmd)
	if err != nil {
		return err
	}

	// Merge config defaults
	if cfg, err := config.LoadDefaults(shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil {
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

	if shared.Verbose {
		prettyReq, _ := json.MarshalIndent(req, "", "  ")
		fmt.Printf("Request:\n%s\n\n", string(prettyReq))
	}

	// Resolve provider (named provider > global > builtin)
	p := shared.ResolveProvider(ProviderNameVideo)

	// Strategy table: first match wins, last entry is the default.
	vctx := &videoDispatchCtx{
		isOpenRouter: p.ProviderType == provider.OpenRouter,
		isYunwu:      p.ProviderType == provider.Yunwu,
	}
	for _, s := range videoStrategies {
		if s.match(req, vctx) {
			saved, err := s.run(req)
			if err == nil && vidGIF {
				if conv, cerr := convertSavedToGIF(saved); cerr != nil {
					fmt.Fprintf(os.Stderr, "Warning: GIF conversion failed: %v\n", cerr)
				} else {
					saved = conv
				}
			}
			if err == nil && vidPreview {
				for _, f := range saved {
					if e := service.PreviewFile(f); e != nil {
						fmt.Fprintf(os.Stderr, "Warning: preview failed: %v\n", e)
					}
				}
			}
			return err
		}
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
	f.StringArrayVarP(&vidImageURLs, "image-url", "i", nil, "Image input: URL or local file path (repeatable); with --gif and no --prompt, converts a local video to GIF")
	f.StringVar(&vidFirstFrame, "first-frame", "", "First frame image URL or local path")
	f.StringVar(&vidLastFrame, "last-frame", "", "Last frame image URL or local path")
	f.StringArrayVar(&vidVideoURLs, "video-url", nil, "Reference video URL (repeatable)")
	f.StringArrayVar(&vidAudioURLs, "audio-url", nil, "Reference audio URL (repeatable)")
	f.StringArrayVar(&vidTools, "tool", nil, "Tool type (e.g. web_search, repeatable)")
	f.BoolVar(&vidRemix, "remix", false, "VEO3 Remix mode: extend video from 8s to 15s (requires --task-id)")
	f.BoolVar(&vidRaw, "raw", false, "Remix: return only the extended portion (VEO3 remix only)")
	f.StringVar(&vidTaskID, "task-id", "", "Original video task ID for remix (required with --remix)")
	f.BoolVar(&vidDryRun, "dry-run", false, "Print request parameters without calling API")
	f.BoolVar(&vidPreview, "preview", false, "Open generated video with system default player")
	f.BoolVar(&vidGIF, "gif", false, "Convert generated videos to GIF after download, or convert a local video via -i/--image-url")
	f.IntVar(&vidGIFWidth, "gif-width", 160, "GIF output width in px (height auto, even)")
	f.StringVar(&vidFFmpegFlags, "ffmpeg-flags", "", `Extra ffmpeg flags appended after the GIF filter (expert escape hatch)`)
	f.StringVar(&shared.JSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	f.StringVar(&vidJobID, "job-id", "", "Resume an OpenRouter video job by ID (loads saved job info and downloads the result)")

	rootCmd.AddCommand(videoCmd)
}
