package audio

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
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

// audioEndpoint resolves the base URL and API key a real audio request uses,
// honoring a named provider (--provider / defaults.audio.provider). Dry-run
// previews must use this, not the package-global Shared.APIBase, so they point
// at the same endpoint the request would actually hit.
func audioEndpoint() (base, key string) {
	p := options.Shared.ResolveProvider(options.ProviderNameAudio)
	base, key = options.Shared.APIBase, options.Shared.APIKey
	if p != nil {
		if p.BaseURL != "" {
			base = p.BaseURL
		}
		if p.APIKey != "" {
			key = p.APIKey
		}
	}
	return client.NormalizeBaseURL(base), key
}

// buildAudioSpeechCurl generates a dry-run curl command for TTS requests.
func buildAudioSpeechCurl(req *types.AudioSpeechRequest) string {
	base, key := audioEndpoint()
	payload, _ := json.MarshalIndent(req, "", "  ")
	var b strings.Builder
	fmt.Fprintf(&b, "curl %s/audio/speech \\\n", base)
	fmt.Fprintf(&b, "  -H \"Authorization: Bearer %s\" \\\n", service.MaskKey(key))
	b.WriteString("  -H \"Content-Type: application/json\" \\\n")
	fmt.Fprintf(&b, "  -d '%s' \\\n", string(payload))
	fmt.Fprintf(&b, "  --output speech.%s\n", req.ResponseFormat)
	return b.String()
}

// buildAudioTranscribeCurl generates a dry-run curl command for base64 (JSON) STT.
func buildAudioTranscribeCurl(req *types.AudioTranscribeRequest) string {
	base, key := audioEndpoint()
	payload, _ := json.MarshalIndent(req, "", "  ")
	var b strings.Builder
	fmt.Fprintf(&b, "curl %s/audio/transcriptions \\\n", base)
	fmt.Fprintf(&b, "  -H \"Authorization: Bearer %s\" \\\n", service.MaskKey(key))
	b.WriteString("  -H \"Content-Type: application/json\" \\\n")
	fmt.Fprintf(&b, "  -d '%s'\n", string(payload))
	return b.String()
}

// buildAudioTranscribeFileCurl generates a dry-run curl command for multipart
// file STT.
func buildAudioTranscribeFileCurl(input, model, language string) string {
	base, key := audioEndpoint()
	var b strings.Builder
	fmt.Fprintf(&b, "curl %s/audio/transcriptions \\\n", base)
	fmt.Fprintf(&b, "  -H \"Authorization: Bearer %s\" \\\n", service.MaskKey(key))
	fmt.Fprintf(&b, "  -F file=\"@%s\" \\\n", input)
	fmt.Fprintf(&b, "  -F model=\"%s\"", model)
	if language != "" {
		fmt.Fprintf(&b, " \\\n  -F language=\"%s\"", language)
	}
	b.WriteString("\n")
	return b.String()
}
