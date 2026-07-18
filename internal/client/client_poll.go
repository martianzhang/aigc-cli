package client

import (
	"encoding/json"
	"fmt"
	"github.com/martianzhang/aigc-cli/internal/types"
	"io"
	"net/http"
	"os"
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
func (c *Client) GetTask(taskID string) (*types.TaskData, error) {
	path := fmt.Sprintf(taskPath, taskID)

	httpReq, err := http.NewRequestWithContext(c.requestContext(), http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("task query failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read task response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("task query returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var taskResp types.TaskResponse
	if err := json.Unmarshal(respBody, &taskResp); err != nil {
		return nil, fmt.Errorf("failed to parse task response: %w", err)
	}

	if taskResp.Code != 200 {
		return nil, fmt.Errorf("task query returned code %d", taskResp.Code)
	}

	return taskResp.Data, nil
}
