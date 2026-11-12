package audio

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/reqbuild"
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
			// stdin is user-supplied prompt/audio; the caller controls the pipe size.
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
func audioEndpoint(cmdName string) (base, key string) {
	p := options.Shared.ResolveProvider(cmdName)
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
	base, key := audioEndpoint(options.ProviderNameAudioSpeak)
	plan := &reqbuild.Plan{
		Method:    http.MethodPost,
		URL:       base + "/audio/speech",
		Body:      req,
		ExtraArgs: []string{"--output speech." + req.ResponseFormat},
	}
	curl, err := plan.RenderCurlsErr(key)
	if err != nil {
		return fmt.Sprintf("# failed to render preview: %v", err)
	}
	return curl
}

// buildAudioTranscribeCurl generates a dry-run curl command for base64 (JSON) STT.
func buildAudioTranscribeCurl(req *types.AudioTranscribeRequest) string {
	base, key := audioEndpoint(options.ProviderNameAudioTranscribe)
	plan := &reqbuild.Plan{Method: http.MethodPost, URL: base + "/audio/transcriptions", Body: req}
	curl, err := plan.RenderCurlsErr(key)
	if err != nil {
		return fmt.Sprintf("# failed to render preview: %v", err)
	}
	return curl
}

// buildAudioTranscribeFileCurl generates a dry-run curl command for multipart
// file STT.
func buildAudioTranscribeFileCurl(input, model, language string) string {
	base, key := audioEndpoint(options.ProviderNameAudioTranscribe)
	plan := &reqbuild.Plan{
		Method:    http.MethodPost,
		URL:       base + "/audio/transcriptions",
		Multipart: true,
		FormFiles: []reqbuild.FormFile{{Field: "file", Path: input}},
		FormFields: []reqbuild.FormField{
			{Name: "model", Value: model},
		},
	}
	if language != "" {
		plan.FormFields = append(plan.FormFields, reqbuild.FormField{Name: "language", Value: language})
	}
	curl, err := plan.RenderCurlsErr(key)
	if err != nil {
		return fmt.Sprintf("# failed to render preview: %v", err)
	}
	return curl
}
