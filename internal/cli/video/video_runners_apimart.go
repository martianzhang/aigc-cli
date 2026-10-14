package video

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runAPIMartVideo handles video generation via APIMart async task API.
// Local images are uploaded by videoPlan.applyUploads before dispatch.
func runAPIMartVideo(req *types.VideoGenerateRequest) (*RunResult, error) {
	c := options.NewClient("video")
	options.ApplyTimeout(c, "video", client.VideoTimeout)
	resp, err := c.VideoSubmit(req)
	if err != nil {
		return nil, fmt.Errorf("submission failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("submission returned no tasks")
	}

	task := resp.Data[0]
	fmt.Printf("Provider: %s\n", options.Shared.ResolveProvider("video").ProviderType)
	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Response code: %d\n", resp.Code)
	fmt.Printf("Task ID: %s\n", task.TaskID)
	fmt.Printf("Status: %s\n\n", task.Status)

	fmt.Println("Polling for completion...")
	taskData, err := c.PollTask(task.TaskID)
	if err != nil {
		return nil, fmt.Errorf("polling failed: %w", err)
	}

	if options.Shared.Verbose {
		prettyResult, _ := json.MarshalIndent(taskData, "", "  ")
		fmt.Printf("\nTask result:\n%s\n", string(prettyResult))
	}

	fmt.Println()
	savePromptFile(taskData.ID, req.Prompt)
	out := &RunResult{}
	if taskData.Result != nil && len(taskData.Result.Videos) > 0 {
		out.Videos, err = service.DownloadVideos(taskData.Result.Videos, options.Shared.OutputDir, taskData.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
		}
	}

	// return_last_frame: APIMart 在 result.images 里回传尾帧图，直接下载保存。
	// 未请求或未返回时，由 saveLastFrames 在调用方用 ffmpeg 本地抽帧兜底。
	if wantsLastFrame(req) && taskData.Result != nil {
		if urls := lastFrameURLs(taskData.Result.Images); len(urls) > 0 {
			frames, ferr := service.DownloadLastFrames(urls, options.Shared.OutputDir, taskData.ID)
			if ferr != nil {
				fmt.Fprintf(os.Stderr, "Warning: last-frame download failed: %v\n", ferr)
			} else {
				out.LastFrames = frames
			}
		}
	}

	fmt.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
		taskData.ActualTime, taskData.Cost, taskData.CreditsCost)
	return out, nil
}
