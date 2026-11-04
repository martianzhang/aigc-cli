package chat

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// Provider /models probing is a fallback for models that are not in the
// built-in table. It is bounded to two seconds and cached so a chat startup
// never waits on a slow model catalogue twice.
const (
	modelContextWindowProbeTimeout = 2 * time.Second
	modelContextWindowCacheTTL     = time.Hour
	modelContextWindowNegativeTTL  = 5 * time.Minute
)

type modelContextCacheEntry struct {
	window  int
	expires time.Time
}

var (
	modelContextCacheMu sync.Mutex
	modelContextCache   = map[string]modelContextCacheEntry{}
)

// autoDetectContextWindow reports whether unknown models may be probed via
// GET /models. Defaults to true; set defaults.chat.auto_detect_context_window:
// false to disable all probing.
func autoDetectContextWindow(cfg *types.ChatDefaults) bool {
	if cfg != nil && cfg.AutoDetectContextWindow != nil {
		return *cfg.AutoDetectContextWindow
	}
	return true
}

// detectModelContextWindowFn is a test seam for the resolution-chain tests.
var detectModelContextWindowFn = detectModelContextWindow

// detectModelContextWindow probes the configured chat provider for model's
// context_length. It returns 0 on any failure; callers fall back to "unknown".
func detectModelContextWindow(model string) int {
	if strings.TrimSpace(model) == "" {
		return 0
	}
	p := options.Shared.ResolveProvider("chat")
	if p == nil {
		return 0
	}
	// /models with a context_length is an OpenAI-compatible convention.
	// Skip native Anthropic/Gemini/local providers instead of waiting for a
	// predictable 401/404.
	switch p.Type {
	case types.ProviderOpenAI, types.ProviderOpenAIChatCompletions, types.ProviderOpenAIResponses, "":
	default:
		return 0
	}
	return probeModelContextWindow(client.NewFromProvider(p), model, modelContextWindowProbeTimeout)
}

// probeModelContextWindow is the testable core. The cache key includes the
// provider base URL so switching providers cannot return another provider's
// window for the same model id.
func probeModelContextWindow(c *client.Client, model string, timeout time.Duration) int {
	if c == nil || strings.TrimSpace(model) == "" || c.BaseURL() == "" {
		return 0
	}
	if timeout <= 0 {
		timeout = modelContextWindowProbeTimeout
	}
	key := strings.TrimRight(c.BaseURL(), "/") + "\x00" + model
	if window, ok := loadModelContextCache(key); ok {
		return window
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	models, err := c.WithContext(ctx).ListModelsOpenAI()
	if err != nil {
		storeModelContextCache(key, 0, modelContextWindowNegativeTTL)
		return 0
	}

	if window := findModelContextWindow(models, model); window > 0 {
		storeModelContextCache(key, window, modelContextWindowCacheTTL)
		return window
	}
	storeModelContextCache(key, 0, modelContextWindowNegativeTTL)
	return 0
}

// findModelContextWindow prefers an exact id match. If none exists, a unique
// suffix match (e.g. requested "gpt-4o" vs listed "openai/gpt-4o") is accepted;
// ambiguous suffix matches are rejected when their windows disagree.
func findModelContextWindow(models []types.OpenAIModel, model string) int {
	for _, m := range models {
		if strings.EqualFold(m.ID, model) {
			return m.ContextWindowTokens()
		}
	}

	requested := lastModelSegment(model)
	matches := make([]int, 0, 1)
	for _, m := range models {
		if strings.EqualFold(lastModelSegment(m.ID), requested) {
			if w := m.ContextWindowTokens(); w > 0 {
				matches = append(matches, w)
			}
		}
	}
	if len(matches) == 0 {
		return 0
	}
	first := matches[0]
	for _, w := range matches[1:] {
		if w != first {
			return 0 // ambiguous: do not guess
		}
	}
	return first
}

func lastModelSegment(id string) string {
	if i := strings.LastIndex(id, "/"); i >= 0 {
		return id[i+1:]
	}
	return id
}

func loadModelContextCache(key string) (int, bool) {
	modelContextCacheMu.Lock()
	defer modelContextCacheMu.Unlock()
	entry, ok := modelContextCache[key]
	if !ok {
		return 0, false
	}
	if time.Now().After(entry.expires) {
		delete(modelContextCache, key)
		return 0, false
	}
	return entry.window, true
}

func storeModelContextCache(key string, window int, ttl time.Duration) {
	modelContextCacheMu.Lock()
	defer modelContextCacheMu.Unlock()
	modelContextCache[key] = modelContextCacheEntry{window: window, expires: time.Now().Add(ttl)}
}

// resetModelContextCache is used by tests to isolate cases.
func resetModelContextCache() {
	modelContextCacheMu.Lock()
	defer modelContextCacheMu.Unlock()
	modelContextCache = map[string]modelContextCacheEntry{}
}
