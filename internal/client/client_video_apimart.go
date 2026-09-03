package client

import (
	"fmt"
	"net/http"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// VideoSubmit sends a video generation request and returns the task submission.
func (c *Client) VideoSubmit(req *types.VideoGenerateRequest) (*types.VideoGenerateResponse, error) {
	var result types.VideoGenerateResponse
	if err := c.doJSON(http.MethodPost, videoSubmitPath, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// VideoRemixSubmit sends a VEO3 remix request to POST /v1/videos/{task_id}/remix.
func (c *Client) VideoRemixSubmit(taskID string, req *types.VideoRemixRequest) (*types.VideoRemixResponse, error) {
	path := fmt.Sprintf("/videos/%s/remix", taskID)
	var result types.VideoRemixResponse
	if err := c.doJSON(http.MethodPost, path, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
