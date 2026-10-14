package video

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/gif"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// RunResult is a video generation run's outcome: the downloaded video files plus
// any last-frame images saved when the request asked for them
// (return_last_frame). Shared by the CLI and the MCP/agent path.
type RunResult struct {
	Videos     []string // 已保存的视频文件路径
	LastFrames []string // 已保存的尾帧图片路径（return_last_frame 时）
}

// wantsLastFrame reports whether the request asked for a last-frame image.
func wantsLastFrame(req *types.VideoGenerateRequest) bool {
	return req != nil && req.ReturnLastFrame != nil && *req.ReturnLastFrame
}

// saveLastFrames 补全 return_last_frame 的本地落盘：优先使用 provider 返回的
// 尾帧图片（out.LastFrames，已下载）；provider 没有返回时，用 ffmpeg 从每个视频
// 本地抽取尾帧。属于尽力而为：失败只告警，不改变命令成败。
func saveLastFrames(out *RunResult) {
	if out == nil || len(out.LastFrames) > 0 {
		return
	}
	for _, f := range out.Videos {
		if filepath.Ext(f) != ".mp4" {
			continue
		}
		p, err := gif.ExtractLastFrame(gif.ExtractLastFrameOptions{
			Input:   f,
			Verbose: options.Shared.Verbose,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: last-frame extraction failed for %s: %v\n", filepath.Base(f), err)
			continue
		}
		fmt.Printf("Saved: %s\n", p)
		out.LastFrames = append(out.LastFrames, p)
	}
}

// lastFrameURLs 把 provider 为一次视频任务返回的图片结果（APIMart 在
// result.images 里回传尾帧图）拍平成一个 URL 列表。
func lastFrameURLs(images []types.ImageResult) []string {
	var urls []string
	for _, img := range images {
		for _, u := range img.URL {
			if u != "" {
				urls = append(urls, u)
			}
		}
	}
	return urls
}
