package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/martianzhang/apimart-cli/internal/types"
)

// buildAudioSpeechRequest builds an AudioSpeechRequest from flags and stdin/file input.
func buildAudioSpeechRequest() (*types.AudioSpeechRequest, error) {
	// Auto-detect piped stdin when --input is not specified
	src := audioSpeechInput
	if src == "" {
		stat, err := os.Stdin.Stat()
		if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return nil, fmt.Errorf("failed to read stdin: %w", err)
			}
			src = string(data)
		}
	} else {
		data, err := readInput(src)
		if err != nil {
			return nil, fmt.Errorf("failed to read input: %w", err)
		}
		src = string(data)
	}

	req := &types.AudioSpeechRequest{
		Model:          audioSpeechModel,
		Input:          src,
		Voice:          audioSpeechVoice,
		ResponseFormat: audioSpeechFormat,
		Speed:          audioSpeechSpeed,
		Instructions:   audioSpeechInstructions,
	}

	if req.Input == "" {
		return nil, fmt.Errorf("input text is required: set via --input flag, file path, or stdin")
	}

	return req, nil
}

// buildAudioSpeechCurl generates a dry-run curl command for TTS requests.
func buildAudioSpeechCurl(req *types.AudioSpeechRequest) string {
	payload, _ := json.MarshalIndent(req, "", "  ")
	var b strings.Builder
	fmt.Fprintf(&b, "curl %s/audio/speech \\\n", shared.APIBase)
	fmt.Fprintf(&b, "  -H \"Authorization: Bearer %s\" \\\n", maskKey(shared.APIKey))
	b.WriteString("  -H \"Content-Type: application/json\" \\\n")
	fmt.Fprintf(&b, "  -d '%s' \\\n", string(payload))
	fmt.Fprintf(&b, "  --output speech.%s\n", req.ResponseFormat)
	return b.String()
}

// buildAudioTranscribeCurl generates a dry-run curl command for STT requests.
func buildAudioTranscribeCurl(req *types.AudioTranscribeRequest) string {
	payload, _ := json.MarshalIndent(req, "", "  ")
	var b strings.Builder
	fmt.Fprintf(&b, "curl %s/audio/transcriptions \\\n", shared.APIBase)
	fmt.Fprintf(&b, "  -H \"Authorization: Bearer %s\" \\\n", maskKey(shared.APIKey))
	b.WriteString("  -H \"Content-Type: application/json\" \\\n")
	fmt.Fprintf(&b, "  -d '%s'\n", string(payload))
	return b.String()
}

// maskKey returns the last 4 characters of an API key, or "***" if too short.
func maskKey(key string) string {
	if len(key) > 4 {
		return "..." + key[len(key)-4:]
	}
	return "***"
}
