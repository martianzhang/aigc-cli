package client

import (
	"fmt"
	"time"

	"github.com/martianzhang/aigc-cli/internal/terminal"
	"github.com/martianzhang/aigc-cli/internal/types"
)

const (
	musicSubmitPath = "/music/generations"
	musicTaskPath   = "/music/tasks/%s"
)

// MusicSubmit sends a music generation request and returns the submission response.
func (c *Client) MusicSubmit(reqBody any) (*types.MusicSubmitResponse, error) {
	var result types.MusicSubmitResponse
	if err := c.doJSON(musicSubmitPath, reqBody, &result); err != nil {
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
func (c *Client) MusicPollTask(taskID string, maxWait time.Duration) (*types.MusicTaskData, error) {
	fmt.Printf("Task submitted: %s\n", taskID)
	fmt.Printf("Waiting %v before first poll...\n", initialDelay)
	select {
	case <-time.After(initialDelay):
	case <-c.requestContext().Done():
		return nil, c.requestContext().Err()
	}

	isTTY := isTerminal()
	prog := terminal.Default()
	prog.StageIfEmpty("music")
	defer prog.Clear()
	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	si := 0
	start := time.Now()
	wait := pollWait(maxWait, maxPollDuration)

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

		if time.Since(start) > wait {
			if isTTY {
				fmt.Println()
			}
			return nil, fmt.Errorf("polling timed out after %v", wait)
		}

		task, err := c.MusicGetTask(taskID)
		if err != nil {
			if isTTY {
				fmt.Println()
			}
			return nil, fmt.Errorf("failed to query task: %w", err)
		}

		progress := min(max(task.Progress, 0), 100)
		prog.Percent(progress)

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
