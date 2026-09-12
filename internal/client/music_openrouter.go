package client

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// openRouterMusicChunk is one SSE data chunk from a streaming music response.
type openRouterMusicChunk struct {
	Choices []struct {
		Delta struct {
			Audio struct {
				Data       string `json:"data"`
				Transcript string `json:"transcript"`
			} `json:"audio"`
		} `json:"delta"`
	} `json:"choices"`
}

// OpenRouterMusicGenerate streams a music generation from OpenRouter's chat
// completions endpoint (modalities text+audio), concatenates the base64 audio
// chunks, and returns the decoded audio bytes plus the transcript.
func (c *Client) OpenRouterMusicGenerate(req *types.OpenRouterMusicRequest) ([]byte, string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(c.requestContext(), http.MethodPost, c.baseURL+chatPath, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	for k, v := range c.defaultHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if isTimeoutError(err) {
			return nil, "", fmt.Errorf("API request timed out: %w\n%s", err, timeoutHint())
		}
		return nil, "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var audioBuf strings.Builder
	var transcript strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk openRouterMusicChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Audio.Data != "" {
				audioBuf.WriteString(choice.Delta.Audio.Data)
			}
			if choice.Delta.Audio.Transcript != "" {
				transcript.WriteString(choice.Delta.Audio.Transcript)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, transcript.String(), fmt.Errorf("failed to read stream: %w", err)
	}
	if audioBuf.Len() == 0 {
		return nil, transcript.String(), fmt.Errorf("no audio data received")
	}

	decoded, err := base64.StdEncoding.DecodeString(audioBuf.String())
	if err != nil {
		return nil, transcript.String(), fmt.Errorf("failed to decode audio data: %w", err)
	}
	return decoded, transcript.String(), nil
}
