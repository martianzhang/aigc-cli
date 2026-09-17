package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/cli/image"
	"github.com/martianzhang/aigc-cli/internal/cli/music"
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/cli/video"
	"github.com/martianzhang/aigc-cli/internal/depth"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// executeGenerateImage runs image generation and returns a text summary for the LLM.
// Uses defaults.image.model from config, NOT the chat model (options.Shared.Model).
func executeGenerateImage(argsJSON string) string {
	var args generateImageArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}

	req := &types.GenerateRequest{
		Prompt:  args.Prompt,
		Size:    args.Size,
		Quality: args.Quality,
	}
	if args.N > 0 {
		v := args.N
		req.N = &v
	}

	// Show actual config defaults that will be applied
	d := options.ImageDefaults()
	hasCfg := d != nil
	if hasCfg {
		var overrides []string
		if d.Model != "" {
			overrides = append(overrides, fmt.Sprintf("model=%s", d.Model))
		}
		if d.Quality != "" {
			overrides = append(overrides, fmt.Sprintf("quality=%s", d.Quality))
		}
		if d.Size != "" {
			overrides = append(overrides, fmt.Sprintf("size=%s", d.Size))
		}
		if d.Resolution != "" {
			overrides = append(overrides, fmt.Sprintf("resolution=%s", d.Resolution))
		}
		if len(overrides) > 0 {
			fmt.Fprintf(options.Stderr(), "\r\n[config] %s\r\n", strings.Join(overrides, " | "))
		}
	}

	// Use shared generation function (same logic as aigc-cli image) with an
	// image-scoped client, not the chat client.
	c := options.NewClient(options.ProviderNameImage)
	saved, err := image.GenerateAndSave(c, req)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return fmt.Sprintf("Successfully generated %d image(s).\nImages saved locally:\n  %s\nUser can use /preview to view them.", len(saved), strings.Join(saved, "\n  "))
}

// executeGenerateVideo runs video generation and returns a text summary for the LLM.
// Uses defaults.video.model from config, NOT the chat model (options.Shared.Model).
func executeGenerateVideo(argsJSON string) string {
	var args generateVideoArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}

	req := &types.VideoGenerateRequest{
		Prompt: args.Prompt,
	}
	if args.Duration > 0 {
		v := args.Duration
		req.Duration = &v
	}
	if args.Resolution != "" {
		req.Resolution = args.Resolution
	}

	// Use shared generation function (same logic as aigc-cli video); the video
	// strategy runner creates its own video-scoped client internally.
	saved, err := video.GenerateAndSave(req)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return fmt.Sprintf("Successfully generated %d video(s).\nFiles saved locally:\n  %s\nUser can use /preview to view them.", len(saved), strings.Join(saved, "\n  "))
}

// executeGenerateMusic runs music generation and returns a text summary for the LLM.
// Uses defaults.music from config, NOT the chat model (options.Shared.Model).
func executeGenerateMusic(argsJSON string) string {
	var args generateMusicArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}

	req := &types.MusicGenerateRequest{
		Prompt: args.Prompt,
		Model:  args.Model,
	}
	if args.Duration > 0 {
		v := args.Duration
		req.Duration = &v
	}
	if args.Instrumental {
		v := true
		req.Instrumental = &v
	}

	// Use shared generation function with a music-scoped client, not the chat client.
	c := options.NewClient(options.ProviderNameMusic)
	saved, err := music.GenerateAndSave(c, req)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return fmt.Sprintf("Successfully generated %d music file(s).\nFiles saved locally:\n  %s\nUser can use /preview to play them.", len(saved), strings.Join(saved, "\n  "))
}

// executeConvertDepth converts an image or video to a grayscale depth map
// (input type auto-detected by extension).
func executeConvertDepth(argsJSON string) string {
	var a struct {
		InputPath  string `json:"input_path"`
		OutputPath string `json:"output_path"`
		StartTime  string `json:"start_time"`
		EndTime    string `json:"end_time"`
		Model      string `json:"model"`
		Invert     bool   `json:"invert"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if a.InputPath == "" {
		return "Error: input_path is required"
	}
	if _, err := os.Stat(a.InputPath); err != nil {
		return fmt.Sprintf("Error: input file not found: %v", err)
	}

	// Resolve output path into the shared output dir.
	outPath := a.OutputPath
	if outPath == "" {
		ext := filepath.Ext(a.InputPath)
		stem := strings.TrimSuffix(filepath.Base(a.InputPath), ext)
		outPath = filepath.Join(options.Shared.OutputDir, stem+"_depth"+ext)
	}

	if service.IsImageInput(a.InputPath) {
		out, err := depth.ConvertImage(depth.ImageOptions{
			Input:   a.InputPath,
			Output:  outPath,
			ModelID: a.Model,
			Invert:  a.Invert,
			Verbose: options.Shared.Verbose,
		})
		if err != nil {
			return fmt.Sprintf("Error: conversion failed: %v\nRun 'aigc-cli depth init' if the depth model is missing.", err)
		}
		return fmt.Sprintf("Depth image saved: %s\nUser can use /preview to view it.\nTip: upload this depth map plus a reference photo to a depth-guided image-to-video platform (e.g. Wan VACE, Kling Motion Control) to generate new content keeping the original structure with the new appearance.", out)
	}

	out, err := depth.Convert(depth.ConvertOptions{
		Input:      a.InputPath,
		Output:     outPath,
		ModelID:    a.Model,
		StartTime:  a.StartTime,
		EndTime:    a.EndTime,
		Invert:     a.Invert,
		Smooth:     true,
		Verbose:    options.Shared.Verbose,
		OnProgress: func(done, total int, fps float64) {},
	})
	if err != nil {
		return fmt.Sprintf("Error: conversion failed: %v\nRun 'aigc-cli depth init' if the depth model is missing.", err)
	}
	return fmt.Sprintf("Depth video saved: %s\nUser can use /preview to view it.\nTip: upload this depth video plus a reference photo to a depth-guided image-to-video platform (e.g. Wan VACE, Kling Motion Control) to generate a new video keeping the original motion with the new appearance.", out)
}
