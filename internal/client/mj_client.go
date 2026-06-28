// Package client implements API client for Midjourney API endpoints.
package client

import (
	"fmt"
	"net/http"
	"time"

	"github.com/martianzhang/apimart-cli/internal/types"
)

const (
	mjSubmitPath = "/midjourney/generations/" // suffix: imagine, blend, upscale, etc.
	mjTaskPath   = "/midjourney/%s"
)

// MidjourneySubmit sends a request to any POST /v1/midjourney/generations/{action} endpoint.
func (c *Client) MidjourneySubmit(action string, reqBody any) (*types.MJSubmitResponse, error) {
	path := mjSubmitPath + action
	var result types.MJSubmitResponse
	if err := c.doJSON(http.MethodPost, path, reqBody, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// MidjourneyGetTask retrieves a Midjourney task by ID using the MJ-specific endpoint.
// The MJ endpoint returns the task object directly (not wrapped in {code, data}).
func (c *Client) MidjourneyGetTask(taskID string) (*types.MJTaskData, error) {
	path := fmt.Sprintf(mjTaskPath, taskID)
	var task types.MJTaskData
	if err := c.doGet(path, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// MidjourneyPollTask polls an MJ task until completion, failure, or MODAL state.
// Returns the task data regardless of terminal state (caller checks .Status).
func (c *Client) MidjourneyPollTask(taskID string) (*types.MJTaskData, error) {
	fmt.Printf("Task submitted: %s\n", taskID)
	fmt.Printf("Waiting %v before first poll...\n", initialDelay)
	time.Sleep(initialDelay)

	isTTY := isTerminal()
	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	si := 0
	start := time.Now()

	if isTTY {
		fmt.Print("  Progress: 0% ")
	}

	for {
		if time.Since(start) > maxPollDuration {
			if isTTY {
				fmt.Println()
			}
			return nil, fmt.Errorf("polling timed out after %v", maxPollDuration)
		}

		task, err := c.MidjourneyGetTask(taskID)
		if err != nil {
			if isTTY {
				fmt.Println()
			}
			return nil, fmt.Errorf("failed to query task: %w", err)
		}

		// Parse progress from string like "100%" or "50%"
		progress := 0
		if task.Progress != "" {
			fmt.Sscanf(task.Progress, "%d%%", &progress)
		}

		if isTTY {
			bar := progressBar(progress, 20)
			fmt.Printf("\r  %s %s %d%% ", spinner[si%len(spinner)], bar, progress)
			si++
		} else {
			fmt.Printf("  Status: %s, Progress: %s\n", task.Status, task.Progress)
		}

		switch task.Status {
		case "SUCCESS", "FAILURE":
			if isTTY {
				fmt.Println()
			}
			return task, nil
		case "MODAL":
			// MODAL is a valid non-terminal state — caller should check this
			if isTTY {
				fmt.Println()
			}
			return task, nil
		default:
			// SUBMITTED, IN_PROGRESS, NOT_START — keep polling
		}

		time.Sleep(pollInterval)
	}
}
