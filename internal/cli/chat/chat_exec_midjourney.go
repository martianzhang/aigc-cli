package chat

import (
	"encoding/json"
	"fmt"

	"github.com/martianzhang/aigc-cli/internal/cli/midjourney"
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func executeMidjourney(c *client.Client, toolName, argsJSON string) string {
	mjClient := midjourney.NewClient()
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
		if mj := options.MidjourneyDefaults(); mj != nil {
			mj.MergeIntoImagine(mjReq)
		}
		text, err := midjourney.SubmitAndGetText(mjClient, "imagine", mjReq)
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
		text, err := midjourney.SubmitAndGetText(mjClient, "describe", req)
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
		text, err := midjourney.SubmitAndGetText(mjClient, "reroll", req)
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
		text, err := midjourney.SubmitAndGetText(mjClient, "video", req)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return text
	}
	return "Error: unknown midjourney tool"
}
