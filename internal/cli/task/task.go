// Package task implements the `aigc-cli task` command.
package task

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// Deps carries the runtime configuration and callbacks the task command needs.
// Its provider is resolved lazily via ResolveProvider (after flags/config are
// loaded) so `--provider` selects the account/base that is queried.
type Deps struct {
	ResolveProvider func(string) *provider.EffectiveProvider
	Providers       map[string]*types.NamedProvider
	DownloadImages  func([]types.ImageResult, string) ([]string, error)
	DownloadVideos  func([]types.VideoResult, string) ([]string, error)
}

// QueryText queries a task by ID and returns a text summary, downloading
// results when available.
func QueryText(d Deps, taskID string) (string, error) {
	var p *provider.EffectiveProvider
	if d.ResolveProvider != nil {
		p = d.ResolveProvider("task")
	}

	if p != nil && p.ProviderType == provider.OpenRouter {
		return "", fmt.Errorf("task query is not available on OpenRouter — use 'aigc-cli video --job-id %s' instead", taskID)
	}
	if p == nil || p.ProviderType != provider.APIMart {
		return "", fmt.Errorf("task query is only supported on APIMart-compatible providers (apimart.ai / apib.ai / aiuxu.com / aishuch.com)")
	}

	if err := options.RequireAPIKey("task", p, d.Providers); err != nil {
		return "", err
	}

	c := client.New(p.APIKey, p.BaseURL, p.HTTPProxy)
	task, err := c.GetTask(taskID)
	if err != nil {
		return "", fmt.Errorf("failed to query task: %w", err)
	}

	msg := fmt.Sprintf("Task %s\nStatus: %s | Progress: %d%%", taskID, task.Status, task.Progress)
	if task.Status == "completed" {
		msg += fmt.Sprintf("\nCost: $%.5f (%.4f credits) | Time: %ds", task.Cost, task.CreditsCost, task.ActualTime)
	}
	if task.Error != nil {
		msg += fmt.Sprintf("\nError: %s", task.Error.Message)
	}

	if task.Result != nil && len(task.Result.Images) > 0 && task.Status == "completed" {
		if saved, err := d.DownloadImages(task.Result.Images, task.ID); err == nil {
			msg += fmt.Sprintf("\nImages saved: %d file(s)", len(saved))
		}
	}
	if task.Result != nil && len(task.Result.Videos) > 0 && task.Status == "completed" {
		if saved, err := d.DownloadVideos(task.Result.Videos, task.ID); err == nil {
			msg += fmt.Sprintf("\nVideos saved: %d file(s)", len(saved))
		}
	}
	return msg, nil
}

// NewCommand builds the `task` cobra command. deps is called at run time so it
// observes flag/config values loaded by the root command.
func NewCommand(deps func() Deps) *cobra.Command {
	return &cobra.Command{
		Use:          "task <task-id>",
		Short:        "Query task status and result",
		SilenceUsage: true,
		Long: `Query the execution status and result of an asynchronous task.

You can query any task by its ID, including image and video generation tasks.

Example:
  aigc-cli task task_01KV4KD9FBH3AZ4DE18A7Y17S3`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			text, err := QueryText(deps(), args[0])
			if err != nil {
				return err
			}
			fmt.Println(text)
			return nil
		},
	}
}
