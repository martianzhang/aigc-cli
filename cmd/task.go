package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/client"
)

// taskCmd represents the `task` command.
var taskCmd = &cobra.Command{
	Use:   "task <task-id>",
	Short: "Query task status and result",
	Long: `Query the execution status and result of an asynchronous task.

You can query any task by its ID, including image and video generation tasks.

Example:
  apimart-cli task task_01KV4KD9FBH3AZ4DE18A7Y17S3`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := args[0]

		c := client.New(apiKey, apiBase, httpProxy)
		task, err := c.GetTask(taskID)
		if err != nil {
			return fmt.Errorf("failed to query task: %w", err)
		}

		pretty, _ := json.MarshalIndent(task, "", "  ")
		fmt.Println(string(pretty))

		// Also download images if available and --output is set
		if task.Result != nil && len(task.Result.Images) > 0 && task.Status == "completed" {
			return downloadImages(task.Result.Images)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(taskCmd)
}
