package client

import (
	"fmt"
	"net/http"
	"time"

	"github.com/martianzhang/aigc-cli/internal/types"
)

const (
	musicSubmitPath = "/music/generations"
	musicTaskPath   = "/music/tasks/%s"
)

// MusicSubmit sends a music generation request and returns the submission response.
func (c *Client) MusicSubmit(reqBody any) (*types.MusicSubmitResponse, error) {
	var result types.MusicSubmitResponse
	if err := c.doJSON(http.MethodPost, musicSubmitPath, reqBody, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// MusicGetTask retrieves a music task by ID using the music-specific task path.
func (c *Client) MusicGetTask(taskID string) (*types.MusicTaskData, error) {
	path := fmt.Sprintf(musicTaskPath, taskID)
	var resp types.MusicTaskResponse
	if err := c.doGet(path, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// MusicPollTask polls a music task until completion or failure.
// Success on completed/success/succeeded; failure on failed/failure.
func (c *Client) MusicPollTask(taskID string) (*types.MusicTaskData, error) {
	fmt.Printf("Task submitted: %s\n", taskID)
	fmt.Printf("Waiting %v before first poll...\n", initialDelay)
	select {
	case <-time.After(initialDelay):
	case <-c.requestContext().Done():
		return nil, c.requestContext().Err()
	}

	isTTY := isTerminal()
	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	si := 0
	start := time.Now()

	if isTTY {
		fmt.Print("  Progress: 0% ")
	}

	for {
		select {
		case <-c.requestContext().Done():
			if isTTY {
				fmt.Println()
			}
			return nil, c.requestContext().Err()
		default:
		}

		if time.Since(start) > maxPollDuration {
			if isTTY {
				fmt.Println()
			}
			return nil, fmt.Errorf("polling timed out after %v", maxPollDuration)
		}

		task, err := c.MusicGetTask(taskID)
		if err != nil {
			if isTTY {
				fmt.Println()
			}
			return nil, fmt.Errorf("failed to query task: %w", err)
		}

		progress := task.Progress
		if progress < 0 {
			progress = 0
		}
		if progress > 100 {
			progress = 100
		}

		if isTTY {
			bar := progressBar(progress, 20)
			fmt.Printf("\r  %s %s %d%% ", spinner[si%len(spinner)], bar, progress)
			si++
		} else {
			fmt.Printf("  Status: %s, Progress: %d%%\n", task.Status, progress)
		}

		switch task.Status {
		case "completed", "success", "succeeded":
			if isTTY {
				fmt.Println()
			}
			return task, nil
		case "failed", "failure":
			if isTTY {
				fmt.Println()
			}
			if task.Error != nil && task.Error.Message != "" {
				return task, fmt.Errorf("task failed: %s", task.Error.Message)
			}
			return task, fmt.Errorf("task failed")
		}

		select {
		case <-time.After(pollInterval):
		case <-c.requestContext().Done():
			if isTTY {
				fmt.Println()
			}
			return nil, c.requestContext().Err()
		}
	}
}
