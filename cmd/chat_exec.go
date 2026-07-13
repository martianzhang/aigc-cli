package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/types"
	"github.com/martianzhang/apimart-cli/internal/watermark"
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
	case "remove_watermark":
		return executeRemoveWatermark(args)
	case "add_watermark":
		return executeAddWatermark(args)
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

// --- Ideas, balance, task agent tools ---

type ideasSearchArgs struct {
	Keywords string `json:"keywords"`
	Limit    int    `json:"limit"`
	Random   bool   `json:"random"`
}

type balanceQueryArgs struct {
	Scope string `json:"scope"`
}

type taskQueryArgs struct {
	TaskID string `json:"task_id"`
}
