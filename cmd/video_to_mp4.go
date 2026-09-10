package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/gif"
)

// convertLocalToMP4 转换单个本地媒体文件（GIF/WebP/MOV/…）为 MP4（纯本地，不调 API）。
// 供 video --mp4 -i <file>（无 prompt）路径使用；保留原文件，输出 <stem>.mp4。
func convertLocalToMP4(input string) error {
	if err := ensureFFmpegAvailable(); err != nil {
		return err
	}
	crop, err := parseGIFCropMargin()
	if err != nil {
		return err
	}
	if _, err := filepath.Abs(input); err != nil {
		return fmt.Errorf("invalid input path: %w", err)
	}
	out, err := gif.ToMP4(gif.ToMP4Options{
		Input:      input,
		CropMargin: crop,
		Verbose:    shared.Verbose,
		ExtraArgs:  gif.SplitExtraArgs(vidFFmpegFlags),
	})
	if err != nil {
		return err
	}
	fmt.Printf("Saved: %s\n", out)
	return nil
}
