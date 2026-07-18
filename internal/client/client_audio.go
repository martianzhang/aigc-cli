package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// AudioSpeech sends a text-to-speech request and returns the audio data as bytes
// along with the response Content-Type. The response is raw binary audio, not JSON.
func (c *Client) AudioSpeech(req *types.AudioSpeechRequest) ([]byte, string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(c.requestContext(), http.MethodPost, c.baseURL+audioSpeechPath, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	c.setOpenRouterHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if isTimeoutError(err) {
			return nil, "", fmt.Errorf("TTS request timed out: %w\n%s", err, timeoutHint())
		}
		return nil, "", fmt.Errorf("TTS request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read TTS response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("TTS API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	contentType := resp.Header.Get("Content-Type")
	return respBody, contentType, nil
}

// AudioTranscribe sends a speech-to-text request using JSON body with base64-encoded
// audio and returns the transcribed text with usage statistics.
func (c *Client) AudioTranscribe(req *types.AudioTranscribeRequest) (*types.AudioTranscribeResponse, error) {
	var result types.AudioTranscribeResponse
	if err := c.doJSON(http.MethodPost, audioTranscribePath, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// AudioTranscribeMultipart sends a speech-to-text request using multipart/form-data,
// compatible with the OpenAI SDK file upload format. filePath must be a local audio file.
func (c *Client) AudioTranscribeMultipart(model, filePath, language string) (*types.AudioTranscribeResponse, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer file.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// model field
	if err := w.WriteField("model", model); err != nil {
		return nil, fmt.Errorf("failed to write model field: %w", err)
	}

	// file field
	part, err := w.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	// optional language field
	if language != "" {
		if err := w.WriteField("language", language); err != nil {
			return nil, fmt.Errorf("failed to write language field: %w", err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(c.requestContext(), http.MethodPost, c.baseURL+audioTranscribePath, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", w.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	c.setOpenRouterHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if isTimeoutError(err) {
			return nil, fmt.Errorf("STT request timed out: %w\n%s", err, timeoutHint())
		}
		return nil, fmt.Errorf("STT request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read STT response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("STT API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result types.AudioTranscribeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse STT response: %w", err)
	}
	return &result, nil
}
