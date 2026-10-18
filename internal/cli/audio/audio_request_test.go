package audio

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// TestBuildAudioSpeechCurl_usesResolvedProvider guards against the dry-run
// preview using the package-global Shared.APIBase: with a named provider it must
// point at that provider's endpoint (and normalized /v1), not the global one.
func TestBuildAudioSpeechCurl_usesResolvedProvider(t *testing.T) {
	old := *options.Shared
	defer func() { *options.Shared = old }()

	options.Shared.Cfg = &types.Config{
		APIKey:  "global-key",
		BaseURL: "https://api.apimart.ai",
		Providers: map[string]*types.NamedProvider{
			"or": {APIKey: "or-key", BaseURL: "https://openrouter.ai/api/v1"},
		},
	}
	options.Shared.APIBase = "https://api.apimart.ai"
	options.Shared.APIKey = "global-key"
	options.Shared.Provider = "or"
	options.Shared.ProviderSet = true

	curl := buildAudioSpeechCurl(&types.AudioSpeechRequest{Model: "m", Voice: "v", ResponseFormat: "mp3"})
	if !strings.Contains(curl, "https://openrouter.ai/api/v1/audio/speech") {
		t.Errorf("curl = %q, want the resolved provider endpoint", curl)
	}
	if strings.Contains(curl, "apimart") {
		t.Errorf("curl = %q, must not use the global base URL", curl)
	}
}

// TestBuildAudioSpeechCurl_normalizesGlobalBase verifies the fallback path also
// normalizes the base URL (bare host gets /v1).
func TestBuildAudioSpeechCurl_normalizesGlobalBase(t *testing.T) {
	old := *options.Shared
	defer func() { *options.Shared = old }()

	options.Shared.Cfg = nil
	options.Shared.APIBase = "https://api.openai.com"
	options.Shared.APIKey = "k"
	options.Shared.APIBaseSet = true
	options.Shared.APIKeySet = true
	options.Shared.Provider = ""
	options.Shared.ProviderSet = false

	curl := buildAudioSpeechCurl(&types.AudioSpeechRequest{Model: "m", Voice: "v", ResponseFormat: "mp3"})
	if !strings.Contains(curl, "https://api.openai.com/v1/audio/speech") {
		t.Errorf("curl = %q, want the normalized global endpoint", curl)
	}
}
