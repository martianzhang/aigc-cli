package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/client"
)

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
		taskID := args[0]

		c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
		task, err := c.GetTask(taskID)
		if err != nil {
			return fmt.Errorf("failed to query task: %w", err)
		}

		if shared.Verbose {
			pretty, _ := json.MarshalIndent(task, "", "  ")
			fmt.Println(string(pretty))
		}

		fmt.Printf("Status: %s | Progress: %d%%", task.Status, task.Progress)
		if task.Status == "completed" {
			fmt.Printf(" | Cost: $%.5f (%.4f credits) | Time: %ds",
				task.Cost, task.CreditsCost, task.ActualTime)
		}
		fmt.Println()

		// Download images if available
		if task.Result != nil && len(task.Result.Images) > 0 && task.Status == "completed" {
			if _, err := downloadImages(task.Result.Images, task.ID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: download error: %v\n", err)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(taskCmd)
}
