package image

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestImageStrategyDispatch_AllProviders(t *testing.T) {
	withImage := &types.GenerateRequest{Model: "m", Prompt: "p", ImageURLs: []string{"a.png"}}
	noImage := &types.GenerateRequest{Model: "m", Prompt: "p"}

	tests := []struct {
		name     string
		req      *types.GenerateRequest
		ctx      *imageDispatchCtx
		wantFunc string
	}{
		{"APIMart uses async task API", noImage, &imageDispatchCtx{isAPIMart: true}, "runAsyncImage"},
		{"OpenRouter uses dedicated image API", noImage, &imageDispatchCtx{isOpenRouter: true}, "runOpenRouterDedicatedImage"},
		{"Agnes uses sync API", noImage, &imageDispatchCtx{isAgnes: true}, "runAgnesImage"},
		{"ModelScope uses async task API", noImage, &imageDispatchCtx{isModelScope: true}, "runModelScopeImage"},
		{"Gemini uses native generateContent API", noImage, &imageDispatchCtx{isGemini: true}, "runGeminiImage"},
		{"ZeekAI image-to-image uses edits JSON", withImage, &imageDispatchCtx{isZeekai: true}, "runImageEditsJSON"},
		{"ZeekAI text-to-image falls through to sync", noImage, &imageDispatchCtx{isZeekai: true}, "runSyncImage"},
		{"Ollama uses native generate endpoint", noImage, &imageDispatchCtx{isOllama: true}, "runOllamaImage"},
		{"OpenLux falls through to generic sync", noImage, &imageDispatchCtx{}, "runSyncImage"},
		{"generic default falls back to sync", noImage, &imageDispatchCtx{}, "runSyncImage"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := firstMatchRunName(tc.req, tc.ctx)
			if !strings.HasSuffix(got, tc.wantFunc) {
				t.Errorf("first match = %q, want suffix %q", got, tc.wantFunc)
			}
		})
	}
}
