package chat

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestBuildChatCurlVerbatim(t *testing.T) {
	options.Shared.APIKey = "test-key"
	options.Shared.APIBase = "https://api.apimart.ai/v1"
	options.Shared.APIKeySet = true
	options.Shared.APIBaseSet = true
	options.Shared.ProviderSet = false
	t.Cleanup(func() {
		options.Shared.APIKeySet = false
		options.Shared.APIBaseSet = false
	})

	const raw = `{"model":"m","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"high"}`
	req := &types.ChatRequest{RawJSON: []byte(raw)}
	p := options.Shared.ResolveProvider(options.ProviderNameChat)

	curl := buildChatCurl(req, p)
	if !strings.Contains(curl, "https://api.apimart.ai/v1/chat/completions") {
		t.Errorf("curl should target the chat endpoint, got:\n%s", curl)
	}
	if !strings.Contains(curl, raw) {
		t.Errorf("curl should forward the --json body verbatim, got:\n%s", curl)
	}
	if !strings.Contains(curl, "...-key") {
		t.Errorf("curl should mask the API key, got:\n%s", curl)
	}
}
