package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/client"
)

// queryTaskText queries a task by ID and returns a text summary.
// Downloads images if available. Shared by CLI and agent loop.
func queryTaskText(taskID string) (string, error) {
	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
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

	// Download images if available
	if task.Result != nil && len(task.Result.Images) > 0 && task.Status == "completed" {
		if saved, err := downloadImages(task.Result.Images, task.ID); err == nil {
			msg += fmt.Sprintf("\nImages saved: %d file(s)", len(saved))
		}
	}
	return msg, nil
}

// taskCmd represents the `task` command.
var taskCmd = &cobra.Command{
	Use:          "task <task-id>",
	Short:        "Query task status and result",
	SilenceUsage: true,
	Long: `Query the execution status and result of an asynchronous task.

You can query any task by its ID, including image and video generation tasks.

Example:
  apimart-cli task task_01KV4KD9FBH3AZ4DE18A7Y17S3`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		text, err := queryTaskText(args[0])
		if err != nil {
			return err
		}
		fmt.Println(text)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(taskCmd)
}
