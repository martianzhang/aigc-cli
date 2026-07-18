package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/watermark"
)

func resolveFileRefs(argsJSON string) string {
	var raw interface{}
	if err := json.Unmarshal([]byte(argsJSON), &raw); err != nil {
		return argsJSON // not valid JSON, return as-is
	}
	resolved := resolveFileRef(raw)
	data, _ := json.Marshal(resolved)
	return string(data)
}

// resolveFileRef recursively walks a parsed JSON value and replaces any
// string starting with "@" by reading the referenced file.
func resolveFileRef(v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		if strings.HasPrefix(val, "@") {
			path := val[1:] // strip the @ prefix
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Sprintf("[Error: cannot read %s: %v]", path, err)
			}
			return string(content)
		}
		return val
	case map[string]interface{}:
		for k, nested := range val {
			val[k] = resolveFileRef(nested)
		}
		return val
	case []interface{}:
		for i, item := range val {
			val[i] = resolveFileRef(item)
		}
		return val
	default:
		return val
	}
}

// executeToolCall executes a single tool call and returns a text result for the LLM.
func executeToolCall(c *client.Client, tc types.ToolCall) string {
	// Expand @filename references in arguments before dispatching.
	// Skip read_file — it handles its own file access.
	args := tc.Function.Arguments
	if tc.Function.Name != "read_file" && strings.Contains(args, `"@`) {
		resolved := resolveFileRefs(args)
		if resolved != args {
			if shared.Verbose {
				fmt.Fprintf(chatStderr, "\r\n[agent] resolved @file refs in %s\r\n", tc.Function.Name)
			}
			args = resolved
		}
	}

	switch tc.Function.Name {
	case "generate_image":
		return executeGenerateImage(c, args)
	case "generate_video":
		return executeGenerateVideo(c, args)
	case "midjourney_imagine", "midjourney_describe", "midjourney_reroll", "midjourney_video":
		return executeMidjourney(c, tc.Function.Name, args)
	case "ideas":
		return executeIdeasSearch(args)
	case "balance":
		return executeBalanceQuery(args)
	case "task":
		return executeTaskQuery(args)
	case "web_fetch":
		return executeWebFetch(args)
	case "grep":
		return executeGrep(args)
	case "read_file":
		return executeReadFile(args)
	case "find":
		return executeFindFiles(args)
	case "remove_watermark":
		return executeRemoveWatermark(args)
	case "add_watermark":
		return executeAddWatermark(args)
	case "generate_speech":
		return executeGenerateSpeech(args)
	case "transcribe_audio":
		return executeTranscribeAudio(args)
	case "describe_image":
		return executeDescribeImage(args)
	default:
		return fmt.Sprintf("Error: unknown tool '%s'", tc.Function.Name)
	}
}

// summarizeToolResult returns a one-line summary of a tool's result for user display.

func summarizeToolResult(toolName, result string) string {
	// Truncate long results to first meaningful line
	firstLine := result
	if idx := strings.IndexAny(result, "\n\r"); idx > 0 {
		firstLine = result[:idx]
	}
	// Strip common prefixes for cleaner display
	firstLine = strings.TrimSpace(firstLine)
	if strings.HasPrefix(firstLine, "Successfully generated") {
		return firstLine
	}
	if strings.HasPrefix(firstLine, "Error:") {
		return firstLine
	}
	if len(firstLine) > 80 {
		firstLine = firstLine[:80] + "..."
	}
	return firstLine
}

// executeGenerateImage runs image generation and returns a text summary for the LLM.
// Uses defaults.image.model from config, NOT the chat model (shared.Model).
func executeGenerateImage(c *client.Client, argsJSON string) string {
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
	hasCfg := shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Image != nil
	if hasCfg {
		d := shared.Cfg.Defaults.Image
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
			fmt.Fprintf(chatStderr, "\r\n[config] %s\r\n", strings.Join(overrides, " | "))
		}
	}

	// Use shared generation function (same logic as aigc-cli image)
	saved, err := generateImageAndSave(c, req)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return fmt.Sprintf("Successfully generated %d image(s).\nImages saved locally:\n  %s\nUser can use /preview to view them.", len(saved), strings.Join(saved, "\n  "))
}

// executeGenerateVideo runs video generation and returns a text summary for the LLM.
// Uses defaults.video.model from config, NOT the chat model (shared.Model).
func executeGenerateVideo(c *client.Client, argsJSON string) string {
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

	// Use shared generation function (same logic as aigc-cli video)
	saved, err := generateVideoAndSave(c, req)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return fmt.Sprintf("Successfully generated %d video(s).\nFiles saved locally:\n  %s\nUser can use /preview to view them.", len(saved), strings.Join(saved, "\n  "))
}

// executeRemoveWatermark runs visible-AI-watermark removal and returns a text summary.
func executeRemoveWatermark(argsJSON string) string {
	var a watermarkArgs
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if a.FilePath == "" {
		return "Error: file_path is required"
	}

	// Auto-load custom watermarks from config directory
	watermark.LoadWatermarkPNGsFromDir(watermarkDir())

	res, err := watermark.RemoveFileHinted(a.FilePath, a.OutputPath, a.Producer)
	if err != nil {
		return fmt.Sprintf("Error: remove failed: %v", err)
	}
	if !res.Removed {
		return "No visible AI watermark detected/removed."
	}
	out := a.OutputPath
	if out == "" {
		ext := filepath.Ext(a.FilePath)
		out = strings.TrimSuffix(a.FilePath, ext) + "_clean" + ext
	}
	return fmt.Sprintf("Successfully removed watermark (engine: %s). Output: %s", res.Name, out)
}

// executeAddWatermark runs visible-AI-watermark addition and returns a text summary.
func executeAddWatermark(argsJSON string) string {
	var a watermarkArgs
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if a.FilePath == "" {
		return "Error: file_path is required"
	}
	if a.Producer == "" {
		return "Error: producer is required (known: gemini, or custom text)"
	}
	res, err := watermark.AddWatermarkFile(a.FilePath, a.OutputPath, a.Producer)
	if err != nil {
		return fmt.Sprintf("Error: add failed: %v", err)
	}
	out := a.OutputPath
	if out == "" {
		ext := filepath.Ext(a.FilePath)
		out = strings.TrimSuffix(a.FilePath, ext) + "_watermarked.png"
	}
	return fmt.Sprintf("Successfully added watermark (engine: %s). Output: %s", res.Name, out)
}

// --- Midjourney agent tools ---

func executeMidjourney(c *client.Client, toolName, argsJSON string) string {
	mjClient := newMJClient()
	// Propagate context for Ctrl+C cancellation
	if mj, ok := mjClient.(*client.Client); ok && c != nil {
		mj.SetContext(c.GetContext())
	}
	switch toolName {
	case "midjourney_imagine":
		var args struct {
			Prompt      string `json:"prompt"`
			ImageURL    string `json:"image_url"`
			AspectRatio string `json:"aspect_ratio"`
			Style       string `json:"style"`
			Version     string `json:"version"`
			Speed       string `json:"speed"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return fmt.Sprintf("Error: invalid arguments: %v", err)
		}
		mjReq := &types.MJImagineRequest{
			Prompt:    args.Prompt,
			ImageURLs: toURLs(args.ImageURL),
			Size:      args.AspectRatio,
			Style:     args.Style,
			Version:   args.Version,
			Speed:     args.Speed,
		}
		// Merge config defaults
		if shared.Cfg != nil && shared.Cfg.Defaults != nil {
			shared.Cfg.Defaults.Midjourney.MergeIntoImagine(mjReq)
		}
		text, err := midjourneySubmitAndGetText(mjClient, "imagine", mjReq)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return text

	case "midjourney_describe":
		var args struct {
			ImageURL string `json:"image_url"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return fmt.Sprintf("Error: invalid arguments: %v", err)
		}
		req := &types.MJDescribeRequest{ImageURLs: toURLs(args.ImageURL)}
		text, err := midjourneySubmitAndGetText(mjClient, "describe", req)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return text

	case "midjourney_reroll":
		var args struct {
			TaskID string `json:"task_id"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return fmt.Sprintf("Error: invalid arguments: %v", err)
		}
		req := &types.MJRerollRequest{TaskID: args.TaskID}
		text, err := midjourneySubmitAndGetText(mjClient, "reroll", req)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return text

	case "midjourney_video":
		var args struct {
			ImageURL string `json:"image_url"`
			Prompt   string `json:"prompt"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return fmt.Sprintf("Error: invalid arguments: %v", err)
		}
		req := &types.MJVideoRequest{ImageURLs: toURLs(args.ImageURL), Prompt: args.Prompt}
		text, err := midjourneySubmitAndGetText(mjClient, "video", req)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return text
	}
	return "Error: unknown midjourney tool"
}

// executeFindFiles finds files by name/pattern under a directory (safe, pure Go).
func executeFindFiles(argsJSON string) string {
	var params struct {
		Pattern   string `json:"pattern"`
		Path      string `json:"path"`
		MaxResult int    `json:"max_results"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if params.Pattern == "" {
		return "Error: pattern is required"
	}
	root := params.Path
	if root == "" {
		root = "."
	}
	maxResults := params.MaxResult
	if maxResults <= 0 {
		maxResults = 30
	} else if maxResults > 100 {
		maxResults = 100
	}

	var results []string
	filepath.Walk(root, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// Skip hidden directories
			if info.Name() != "." && strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if len(results) >= maxResults {
			return filepath.SkipAll
		}
		matched, _ := filepath.Match(params.Pattern, info.Name())
		if !matched {
			matched = strings.Contains(strings.ToLower(info.Name()), strings.ToLower(params.Pattern))
		}
		if matched {
			size := info.Size()
			sizeStr := fmt.Sprintf("%d B", size)
			if size > 1024*1024 {
				sizeStr = fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
			} else if size > 1024 {
				sizeStr = fmt.Sprintf("%.1f KB", float64(size)/1024)
			}
			results = append(results, fmt.Sprintf("%s  (%s, %s)", fpath, sizeStr, info.ModTime().Format("2006-01-02")))
		}
		return nil
	})

	if len(results) == 0 {
		return fmt.Sprintf("No files found matching %q under %s", params.Pattern, root)
	}
	header := fmt.Sprintf("Found %d file(s) matching %q under %s:\n", len(results), params.Pattern, root)
	return header + strings.Join(results, "\n")
}

// executeGenerateSpeech runs TTS and returns a text summary for the LLM.
func executeGenerateSpeech(argsJSON string) string {
	var params struct {
		Input  string `json:"input"`
		Model  string `json:"model"`
		Voice  string `json:"voice"`
		Format string `json:"format"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if params.Input == "" {
		return "Error: input is required"
	}
	if params.Voice == "" {
		return "Error: voice is required"
	}
	model := params.Model
	if model == "" {
		if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Audio != nil && shared.Cfg.Defaults.Audio.SpeakModel != "" {
			model = shared.Cfg.Defaults.Audio.SpeakModel
		} else {
			model = "gpt-4o-mini-tts"
		}
	}
	format := params.Format
	if format == "" {
		format = "mp3"
	}

	req := &types.AudioSpeechRequest{
		Model:          model,
		Input:          params.Input,
		Voice:          params.Voice,
		ResponseFormat: format,
	}

	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
	applyTimeout(c, "audio", client.AudioTimeout)

	audioData, _, err := c.AudioSpeech(req)
	if err != nil {
		return fmt.Sprintf("Error: TTS failed: %v", err)
	}

	filename, err := saveAudioFile(audioData, format)
	if err != nil {
		return fmt.Sprintf("Error: failed to save audio: %v", err)
	}

	return fmt.Sprintf("Speech generated and saved to: %s\nFormat: %s\nSize: %d bytes\nModel: %s\nVoice: %s",
		filename, format, len(audioData), model, params.Voice)
}

// executeTranscribeAudio runs STT and returns a text summary for the LLM.
func executeTranscribeAudio(argsJSON string) string {
	var params struct {
		FilePath string `json:"file_path"`
		Model    string `json:"model"`
		Language string `json:"language"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if params.FilePath == "" {
		return "Error: file_path is required"
	}
	model := params.Model
	if model == "" {
		if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Audio != nil && shared.Cfg.Defaults.Audio.TranscribeModel != "" {
			model = shared.Cfg.Defaults.Audio.TranscribeModel
		} else {
			model = "whisper-1"
		}
	}

	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
	applyTimeout(c, "audio", client.AudioTimeout)

	resp, err := c.AudioTranscribeMultipart(model, params.FilePath, params.Language)
	if err != nil {
		return fmt.Sprintf("Error: STT failed: %v", err)
	}

	result := fmt.Sprintf("Transcription result (model: %s):\n%s", model, resp.Text)
	if resp.Usage != nil && resp.Usage.Cost > 0 {
		result += fmt.Sprintf("\n(Cost: $%.5f)", resp.Usage.Cost)
	}
	return result
}

// describeImageArgs is the JSON structure for describe_image tool arguments.
type describeImageArgs struct {
	FilePath string `json:"file_path"`
	Caption  string `json:"caption,omitempty"`
}

// executeDescribeImage reads or writes the caption of an image file.
func executeDescribeImage(argsJSON string) string {
	var args describeImageArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.FilePath == "" {
		return "Error: file_path is required"
	}

	// Resolve relative path
	path := args.FilePath
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return fmt.Sprintf("Error: invalid path: %v", err)
		}
		path = abs
	}

	// Write mode
	if args.Caption != "" || args.Caption == "" && len(argsJSON) > 0 {
		// Check if "caption" key was actually present in the JSON
		var raw map[string]json.RawMessage
		if json.Unmarshal([]byte(argsJSON), &raw) == nil {
			if _, hasCaption := raw["caption"]; hasCaption {
				if err := service.WriteDescription(path, args.Caption); err != nil {
					return fmt.Sprintf("Error writing caption: %v", err)
				}
				if args.Caption == "" {
					return fmt.Sprintf("Caption cleared for %s", filepath.Base(path))
				}
				return fmt.Sprintf("Caption set: %s", args.Caption)
			}
		}
	}

	// Read mode
	current, err := service.ReadDescription(path)
	if err != nil {
		return fmt.Sprintf("Error reading caption: %v", err)
	}
	if current == "" {
		return fmt.Sprintf("No caption set for %s", filepath.Base(path))
	}
	return fmt.Sprintf("Caption: %s", current)
}
