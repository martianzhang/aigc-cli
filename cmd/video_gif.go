package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/gif"
)

// ensureGIFAvailable 校验 ffmpeg 可用；不可用则返回带安装提示的错误。
// 在 video 提交 API 之前调用，避免为注定失败的转换浪费一次生成费用。
func ensureGIFAvailable() error {
	if !gif.Available() {
		return gif.MissingHint()
	}
	return nil
}

// convertSavedToGIF 把已下载的视频文件转成 GIF，返回替换后的文件列表。
// 每个 .mp4 生成 <stem>_<width>px.gif；非 mp4 文件原样保留。
func convertSavedToGIF(saved []string) ([]string, error) {
	out := make([]string, 0, len(saved))
	for _, f := range saved {
		if filepath.Ext(f) != ".mp4" {
			out = append(out, f)
			continue
		}
		p, err := gif.Convert(gif.ConvertOptions{
			Input:     f,
			Width:     vidGIFWidth,
			Verbose:   shared.Verbose,
			ExtraArgs: gif.SplitExtraArgs(vidFFmpegFlags),
		})
		if err != nil {
			return out, fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
		fmt.Printf("Saved: %s\n", p)
		out = append(out, p)
	}
	return out, nil
}

// convertLocalToGIF 转换单个本地视频文件（纯本地，不调 API）。
// 供 video --gif -i <file>（无 prompt）路径使用。
func convertLocalToGIF(input string) error {
	if err := ensureGIFAvailable(); err != nil {
		return err
	}
	if _, err := filepath.Abs(input); err != nil {
		return fmt.Errorf("invalid input path: %w", err)
	}
	out, err := gif.Convert(gif.ConvertOptions{
		Input:     input,
		Width:     vidGIFWidth,
		Verbose:   shared.Verbose,
		ExtraArgs: gif.SplitExtraArgs(vidFFmpegFlags),
	})
	if err != nil {
		return err
	}
	fmt.Printf("Saved: %s\n", out)
	return nil
}
