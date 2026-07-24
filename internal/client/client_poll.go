package client

import (
	"fmt"
	"os"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func isTerminal() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func progressBar(pct, width int) string {
	filled := pct * width / 100
	var bar string
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	return bar
}

// GetTask retrieves a single task by ID.
// Reuses doGetWithHeaders for timeout hints and context propagation.
func (c *Client) GetTask(taskID string) (*types.TaskData, error) {
	path := fmt.Sprintf(taskPath, taskID)

	var taskResp types.TaskResponse
	headers := map[string]string{}
	if c.apiKey != "" {
		headers["Authorization"] = "Bearer " + c.apiKey
	}
	if err := c.doGetWithHeaders(path, &taskResp, headers); err != nil {
		return nil, err
	}
	if taskResp.Code != 200 {
		return nil, fmt.Errorf("task query failed: (code %d)", taskResp.Code)
	}
	return taskResp.Data, nil
}
