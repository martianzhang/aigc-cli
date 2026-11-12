package music

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/clientfactory"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/reqbuild"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// ============================================================================
// Curl builder
// ============================================================================
func buildMusicCurl(baseURL, apiKey string, body any) string {
	base := strings.TrimRight(baseURL, "/")
	if !client.HasVersionSuffix(base) {
		base += "/v1"
	}
	plan := &reqbuild.Plan{Method: http.MethodPost, URL: base + "/music/generations", Body: body}
	curl, err := plan.RenderCurlsErr(apiKey)
	if err != nil {
		return fmt.Sprintf("# failed to render preview: %v", err)
	}
	return curl
}

// ============================================================================
// Runner + Display
// ============================================================================

// runMusicSubmitAndPoll submits a music body, polls, and displays results.
// --dry-run short-circuits here (mirrors runMJSubmitAndPoll).
func runMusicSubmitAndPoll(c client.APIClient, baseURL, apiKey string, body any) ([]string, error) {
	if musicDryRun {
		ui.Println(buildMusicCurl(baseURL, apiKey, body))
		return nil, nil
	}

	if options.Shared.Verbose {
		prettyReq, _ := json.MarshalIndent(body, "", "  ")
		ui.Printf("Request:\n%s\n\n", string(prettyReq))
	}

	resp, err := c.MusicSubmit(body)
	if err != nil {
		return nil, fmt.Errorf("submission failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("submission returned no tasks")
	}

	task := resp.Data[0]
	ui.Printf("Response code: %d\n", resp.Code)
	ui.Printf("Task ID: %s\n", task.TaskID)
	ui.Printf("Status: %s\n\n", task.Status)

	ui.Println("Polling for completion...")
	taskData, err := c.MusicPollTask(task.TaskID, clientfactory.TaskWait(types.MediaMusic))
	if err != nil {
		return nil, fmt.Errorf("polling failed: %w", err)
	}

	displayMusicResult(taskData)
	if taskData.Result != nil && len(taskData.Result.Music) > 0 {
		saved, err := downloadMusics(taskData.Result.Music, taskData.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to download music: %w", err)
		}
		return saved, nil
	}
	return nil, nil
}

// displayMusicResult prints a music task result in a human-readable format.
func displayMusicResult(task *types.MusicTaskData) {
	if task == nil {
		return
	}

	if options.Shared.Verbose {
		pretty, _ := json.MarshalIndent(task, "", "  ")
		ui.Printf("\nTask result:\n%s\n", string(pretty))
	}

	ui.Println()
	ui.Printf("Status: %s\n", task.Status)

	if task.Error != nil && task.Error.Message != "" {
		ui.Printf("Error: %s\n", task.Error.Message)
		return
	}

	if task.Result != nil {
		for i, track := range task.Result.Music {
			title := track.Title
			if title == "" {
				title = fmt.Sprintf("Track %d", i+1)
			}
			ui.Printf("Track %d: %s\n", i+1, title)
			switch {
			case float64(track.Duration) > 0:
				ui.Printf("  Duration: %.1fs\n", float64(track.Duration))
			case track.DurationSeconds != "":
				ui.Printf("  Duration: %ss\n", track.DurationSeconds)
			}
			if u := musicTrackURL(track); u != "" {
				ui.Printf("  Audio: %s\n", u)
			}
			if track.ImageURL != "" {
				ui.Printf("  Image: %s\n", track.ImageURL)
			}
		}
	}

	if task.Cost > 0 || task.ActualTime > 0 {
		ui.Printf("Completed in %ds | Cost: $%.5f (%.4f credits)\n",
			task.ActualTime, task.Cost, task.CreditsCost)
	}
}

// GenerateAndSave generates music via p and saves the audio files to disk.
// Shared by the CLI and the agent loop.
//
// p must be the provider that built c so the dispatch table and the dry-run
// endpoint describe the same backend. When the caller explicitly selected a
// provider other than defaults.music.provider, that default block is not
// applied: its model (e.g. suno) belongs to a different backend.
func GenerateAndSave(c client.APIClient, p *provider.EffectiveProvider, req *types.MusicGenerateRequest) ([]string, error) {
	if req == nil {
		return nil, fmt.Errorf("music request is required")
	}

	configured := options.Shared.ResolveProvider(options.ProviderNameMusic)
	if p == nil {
		p = configured
	}
	explicitProvider := p.Name != "" && p.Name != configured.Name

	cfg := options.MusicDefaults()
	if cfg == nil {
		if loaded, err := config.Load(options.Shared.CfgFile); err == nil && loaded != nil && loaded.Defaults != nil {
			cfg = loaded.Defaults.Music
		}
	}
	if explicitProvider {
		if req.Model == "" {
			req.Model = p.Model
		}
	} else {
		cfg.MergeIntoMusic(req) // nil-safe
	}

	clientfactory.ApplyTimeout(c, "music", client.MusicTimeout)

	return dispatchMusic(c, req, newMusicDispatchCtx(c, p))
}
