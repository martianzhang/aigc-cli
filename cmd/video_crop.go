package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/gif"
)

// cropSavedVideos 把已下载的视频按 --crop-margin 裁掉水印边。
// 保留每个原始视频文件，并生成 <stem>_crop.mp4；返回原始文件 + 裁剪文件的合并列表。
// 非 mp4 文件原样保留。
func cropSavedVideos(saved []string) ([]string, error) {
	crop, err := parseGIFCropMargin()
	if err != nil {
		return saved, err
	}
	out := make([]string, 0, len(saved)*2)
	for _, f := range saved {
		out = append(out, f) // 保留原视频
		if filepath.Ext(f) != ".mp4" {
			continue
		}
		p, err := gif.CropVideo(gif.CropVideoOptions{
			Input:      f,
			CropMargin: crop,
			Verbose:    shared.Verbose,
		})
		if err != nil {
			return out, fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
		fmt.Printf("Saved: %s\n", p)
		out = append(out, p)
	}
	return out, nil
}

// cropLocalVideo 裁剪单个本地视频文件（纯本地，不调 API）。
// 供 video --crop-margin -i <file>（无 prompt）路径使用；输出 <stem>_crop.mp4，不覆盖原文件。
func cropLocalVideo(input string) error {
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
	out, err := gif.CropVideo(gif.CropVideoOptions{
		Input:      input,
		CropMargin: crop,
		Verbose:    shared.Verbose,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Saved: %s\n", out)
	return nil
}
