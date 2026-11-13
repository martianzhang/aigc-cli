package chat

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/testutil"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestDryRunPreviewMakesNoRequests(t *testing.T) {
	defer testutil.FailOnNetwork(t)()

	p := &provider.EffectiveProvider{Type: types.ProviderOpenAI, BaseURL: "https://example.com/v1"}
	_ = buildChatCurl(&types.ChatRequest{Model: "m", Messages: []types.ChatMessage{{Role: "user", Content: "hi"}}}, p)
}
