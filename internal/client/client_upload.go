package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/martianzhang/aigc-cli/internal/types"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

// UploadImage uploads a local image file and returns the public URL.
func (c *Client) UploadImage(filePath string) (*types.UploadResponse, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(fw, file); err != nil {
		return nil, fmt.Errorf("failed to copy file content: %w", err)
	}
	w.Close()

	httpReq, err := http.NewRequestWithContext(c.requestContext(), http.MethodPost, c.baseURL+uploadPath, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload request: %w", err)
	}
	httpReq.Header.Set("Content-Type", w.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	// Uploads may take longer
	oldTimeout := c.httpClient.Timeout
	c.httpClient.Timeout = uploadTimeout
	defer func() { c.httpClient.Timeout = oldTimeout }()

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read upload response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result types.UploadResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse upload response: %w", err)
	}
	return &result, nil
}

// ResolveLocalImages checks each URL; if it's a local file path, uploads it
// and returns the public URL. Unchanged URLs are returned as-is.
func (c *Client) ResolveLocalImages(urls []string) ([]string, error) {
	resolved := make([]string, len(urls))
	for i, u := range urls {
		if isLocalFile(u) {
			fmt.Printf("  Uploading local file: %s ...\n", u)
			resp, err := c.UploadImage(u)
			if err != nil {
				return nil, fmt.Errorf("failed to upload %s: %w", u, err)
			}
			fmt.Printf("  -> %s\n", resp.URL)
			resolved[i] = resp.URL
		} else {
			resolved[i] = u
		}
	}
	return resolved, nil
}
