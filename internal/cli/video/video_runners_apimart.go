package video

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/clientfactory"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// runAPIMartVideo handles video generation via APIMart async task API.
// Local images are uploaded by videoPlan.applyUploads before dispatch.
func runAPIMartVideo(ctx context.Context, c *client.Client, p *provider.EffectiveProvider, req *types.VideoGenerateRequest) (*RunResult, error) {
	resp, err := c.VideoSubmit(req)
	if err != nil {
		return nil, fmt.Errorf("submission failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("submission returned no tasks")
	}

	task := resp.Data[0]
	ui.Printf("Provider: %s\n", p.ProviderType)
	ui.Printf("Model: %s\n", req.Model)
	ui.Printf("Response code: %d\n", resp.Code)
	ui.Printf("Task ID: %s\n", task.TaskID)
	ui.Printf("Resume: aigc-cli task %s\n", task.TaskID)
	ui.Printf("Status: %s\n\n", task.Status)

	ui.Println("Polling for completion...")
	taskData, err := c.PollTask(task.TaskID, clientfactory.TaskWait(types.MediaVideo))
	if err != nil {
		return nil, fmt.Errorf("polling failed: %w%s", err, service.TaskResume(task.TaskID))
	}

	if options.Shared.Verbose {
		prettyResult, _ := json.MarshalIndent(taskData, "", "  ")
		ui.Printf("\nTask result:\n%s\n", string(prettyResult))
	}

	ui.Println()
	service.SavePromptBestEffort(options.Shared.SavePrompt, options.Shared.OutputDir, "video", taskData.ID, req.Prompt)
	out := &RunResult{}
	if taskData.Result == nil || len(taskData.Result.Videos) == 0 {
		return nil, fmt.Errorf("task completed but returned no videos")
	}
	out.Videos, err = service.DownloadVideos(taskData.Result.Videos, options.Shared.OutputDir, taskData.ID)
	if err != nil {
		return nil, err
	}

	// return_last_frame: APIMart 在 result.images 里回传尾帧图，直接下载保存。
	// 未请求或未返回时，由 saveLastFrames 在调用方用 ffmpeg 本地抽帧兜底。
	if wantsLastFrame(req) && taskData.Result != nil {
		if urls := lastFrameURLs(taskData.Result.Images); len(urls) > 0 {
			frames, ferr := service.DownloadLastFrames(urls, options.Shared.OutputDir, taskData.ID)
			if ferr != nil {
				ui.Eprintf("Warning: last-frame download failed: %v\n", ferr)
			} else {
				out.LastFrames = frames
			}
		}
	}

	ui.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
		taskData.ActualTime, taskData.Cost, taskData.CreditsCost)
	return out, nil
}
