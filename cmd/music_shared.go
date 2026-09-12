package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// ============================================================================
// Curl builder
// ============================================================================
func buildMusicCurl(baseURL, apiKey string, body any) string {
	bodyJSON, _ := json.Marshal(body)
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	if !client.HasVersionSuffix(base) {
		base += "/v1"
	}
	url := base + "/music/generations"

	cmd := fmt.Sprintf("curl -X POST %s \\\n", url)
	cmd += fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", maskKey(apiKey))
	cmd += "  -H \"Content-Type: application/json\" \\\n"
	cmd += fmt.Sprintf("  -d '%s'", string(bodyJSON))
	return cmd
}

// ============================================================================
// Runner + Display
// ============================================================================

// runMusicSubmitAndPoll submits a music body, polls, and displays results.
// --dry-run short-circuits here (mirrors runMJSubmitAndPoll).
func runMusicSubmitAndPoll(c client.APIClient, baseURL, apiKey string, body any) error {
	if musicDryRun {
		fmt.Println(buildMusicCurl(baseURL, apiKey, body))
		return nil
	}

	if shared.Verbose {
		prettyReq, _ := json.MarshalIndent(body, "", "  ")
		fmt.Printf("Request:\n%s\n\n", string(prettyReq))
	}

	resp, err := c.MusicSubmit(body)
	if err != nil {
		return fmt.Errorf("submission failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return fmt.Errorf("submission returned no tasks")
	}

	task := resp.Data[0]
	fmt.Printf("Response code: %d\n", resp.Code)
	fmt.Printf("Task ID: %s\n", task.TaskID)
	fmt.Printf("Status: %s\n\n", task.Status)

	fmt.Println("Polling for completion...")
	taskData, err := c.MusicPollTask(task.TaskID)
	if err != nil {
		return fmt.Errorf("polling failed: %w", err)
	}

	displayMusicResult(taskData)
	if taskData.Result != nil && len(taskData.Result.Music) > 0 {
		if _, err := downloadMusics(taskData.Result.Music, taskData.ID); err != nil {
			return fmt.Errorf("failed to download music: %w", err)
		}
	}
	return nil
}

// displayMusicResult prints a music task result in a human-readable format.
func displayMusicResult(task *types.MusicTaskData) {
	if task == nil {
		return
	}

	if shared.Verbose {
		pretty, _ := json.MarshalIndent(task, "", "  ")
		fmt.Printf("\nTask result:\n%s\n", string(pretty))
	}

	fmt.Println()
	fmt.Printf("Status: %s\n", task.Status)

	if task.Error != nil && task.Error.Message != "" {
		fmt.Printf("Error: %s\n", task.Error.Message)
		return
	}

	if task.Result != nil {
		for i, track := range task.Result.Music {
			title := track.Title
			if title == "" {
				title = fmt.Sprintf("Track %d", i+1)
			}
			fmt.Printf("Track %d: %s\n", i+1, title)
			switch {
			case float64(track.Duration) > 0:
				fmt.Printf("  Duration: %.1fs\n", float64(track.Duration))
			case track.DurationSeconds != "":
				fmt.Printf("  Duration: %ss\n", track.DurationSeconds)
			}
			if u := musicTrackURL(track); u != "" {
				fmt.Printf("  Audio: %s\n", u)
			}
			if track.ImageURL != "" {
				fmt.Printf("  Image: %s\n", track.ImageURL)
			}
		}
	}

	if task.Cost > 0 || task.ActualTime > 0 {
		fmt.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
			task.ActualTime, task.Cost, task.CreditsCost)
	}
}

// generateMusicAndSave generates music via the configured provider and saves
// the audio files to disk. Shared by CLI and the agent loop.
func generateMusicAndSave(c *client.Client, req *types.MusicGenerateRequest) ([]string, error) {
	if req == nil {
		return nil, fmt.Errorf("music request is required")
	}

	cfg := musicDefaults()
	if cfg == nil {
		if loaded, err := config.Load(shared.CfgFile); err == nil && loaded != nil && loaded.Defaults != nil {
			cfg = loaded.Defaults.Music
		}
	}
	cfg.MergeIntoMusic(req) // nil-safe

	applyTimeout(c, "music", client.MusicTimeout)

	if provider.Detect(c.BaseURL()) == provider.OpenRouter {
		orReq := buildOpenRouterMusicReq(req)
		audio, _, err := c.OpenRouterMusicGenerate(orReq)
		if err != nil {
			return nil, fmt.Errorf("music generation failed: %w", err)
		}
		format := "mp3"
		if orReq.Audio != nil && orReq.Audio.Format != "" {
			format = orReq.Audio.Format
		}
		saved, err := saveAudioFile(audio, format)
		if err != nil {
			return nil, fmt.Errorf("failed to save music: %w", err)
		}
		return []string{saved}, nil
	}

	if req.Model == "" {
		req.Model = "suno"
	}

	body, err := buildMusicBody(req)
	if err != nil {
		return nil, err
	}

	resp, err := c.MusicSubmit(body)
	if err != nil {
		return nil, fmt.Errorf("submission failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("submission returned no tasks")
	}

	taskData, err := c.MusicPollTask(resp.Data[0].TaskID)
	if err != nil {
		return nil, fmt.Errorf("polling failed: %w", err)
	}
	if taskData.Result == nil || len(taskData.Result.Music) == 0 {
		return nil, fmt.Errorf("no music in task result")
	}
	return downloadMusics(taskData.Result.Music, taskData.ID)
}
