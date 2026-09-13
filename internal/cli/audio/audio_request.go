package audio

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
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
		data, err := service.ReadInput(src)
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
	fmt.Fprintf(&b, "curl %s/audio/speech \\\n", options.Shared.APIBase)
	fmt.Fprintf(&b, "  -H \"Authorization: Bearer %s\" \\\n", service.MaskKey(options.Shared.APIKey))
	b.WriteString("  -H \"Content-Type: application/json\" \\\n")
	fmt.Fprintf(&b, "  -d '%s' \\\n", string(payload))
	fmt.Fprintf(&b, "  --output speech.%s\n", req.ResponseFormat)
	return b.String()
}

// buildAudioTranscribeCurl generates a dry-run curl command for STT requests.
func buildAudioTranscribeCurl(req *types.AudioTranscribeRequest) string {
	payload, _ := json.MarshalIndent(req, "", "  ")
	var b strings.Builder
	fmt.Fprintf(&b, "curl %s/audio/transcriptions \\\n", options.Shared.APIBase)
	fmt.Fprintf(&b, "  -H \"Authorization: Bearer %s\" \\\n", service.MaskKey(options.Shared.APIKey))
	b.WriteString("  -H \"Content-Type: application/json\" \\\n")
	fmt.Fprintf(&b, "  -d '%s'\n", string(payload))
	return b.String()
}
