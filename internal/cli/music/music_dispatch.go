package music

import (
	"fmt"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// musicDispatchCtx carries the resolved provider identity used to pick a runner.
// baseURL/apiKey are kept for --dry-run so the rendered curl matches the real
// endpoint even though the client already holds them.
type musicDispatchCtx struct {
	providerType provider.Type
	baseURL      string
	apiKey       string
}

// musicStrategy is one dispatch rule. run returns paths of saved audio files.
type musicStrategy struct {
	match func(*musicDispatchCtx) bool
	run   func(client.APIClient, *types.MusicGenerateRequest, *musicDispatchCtx) ([]string, error)
}

// musicStrategies is the ordered dispatch table; first match wins.
// Each provider owns exactly one delivery mode (sync / async / SSE), so there
// is one runner per provider rather than one per (provider × mode).
var musicStrategies = []musicStrategy{
	{match: providerIs(provider.OpenRouter), run: runOpenRouterMusic},
	{match: providerIs(provider.Bailian), run: runFunMusicMusic},
	// Default: APIMart (suno / flowmusic), async submit → poll.
	{match: func(*musicDispatchCtx) bool { return true }, run: runAPIMartMusic},
}

func providerIs(t provider.Type) func(*musicDispatchCtx) bool {
	return func(ctx *musicDispatchCtx) bool { return ctx.providerType == t }
}

// newMusicDispatchCtx identifies the provider from the client's base URL.
func newMusicDispatchCtx(c client.APIClient, p *provider.EffectiveProvider) *musicDispatchCtx {
	ctx := &musicDispatchCtx{providerType: provider.Detect(c.BaseURL())}
	if p != nil {
		ctx.baseURL, ctx.apiKey = p.BaseURL, p.APIKey
	}
	return ctx
}

// dispatchMusic runs the first matching strategy.
func dispatchMusic(c client.APIClient, req *types.MusicGenerateRequest, ctx *musicDispatchCtx) ([]string, error) {
	for _, s := range musicStrategies {
		if s.match(ctx) {
			return s.run(c, req, ctx)
		}
	}
	return nil, fmt.Errorf("no music strategy for provider %s", ctx.providerType)
}

// runAPIMartMusic runs the APIMart async submit/poll path (suno / flowmusic).
func runAPIMartMusic(c client.APIClient, req *types.MusicGenerateRequest, ctx *musicDispatchCtx) ([]string, error) {
	body, err := buildMusicBody(req)
	if err != nil {
		return nil, err
	}
	return runMusicSubmitAndPoll(c, ctx.baseURL, ctx.apiKey, body)
}
