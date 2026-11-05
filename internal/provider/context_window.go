package provider

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// modelContextWindow is the built-in context-window table for mainstream 2026
// model families. It is only a fallback: an explicit --context-size, config
// context_size, or a provider/model override always wins. Unknown models return
// 0; the chat compaction layer then uses fallback_context_window (128K by
// default) instead of guessing.
//
// Last reviewed: 2026-11. Sources: vendor documentation and models.dev.
//
// Entries are family prefixes, not exact model ids. Only families whose
// documented window is at least 128K are listed; legacy and special-purpose
// models are intentionally omitted and can be configured by the user. Matching
// uses the longest matching prefix, so a more specific entry such as
// "deepseek-v4" wins over "deepseek". A provider prefix such as "anthropic/"
// and a variant suffix such as ":free" are stripped before matching.
var modelContextWindow = []struct {
	prefix string
	window int
}{
	// OpenAI (gpt-5.5 and older are retired and intentionally omitted).
	{"gpt-5.6", 1050000},
	{"gpt-6", 1050000},

	// Anthropic
	{"claude", 200000},
	{"claude-opus-4-6", 1000000},
	{"claude-sonnet-4-6", 1000000},
	{"claude-opus-5", 1000000},
	{"claude-sonnet-5", 1000000},

	// Google
	{"gemini", 1048576},
	{"gemma", 262144},

	// DeepSeek
	{"deepseek", 128000},
	{"deepseek-v4", 1048576},

	// Alibaba
	{"qwen", 131072},
	{"qwen-plus", 1000000},
	{"qwen-flash", 1000000},

	// Zhipu (glm-5 and older are retired and intentionally omitted).
	{"glm-5.2", 1000000},
	{"glm-5.3", 1000000},

	// Moonshot / Kimi
	{"kimi", 131072},
	{"kimi-k2", 262144},
	{"kimi-k3", 1048576},

	// ByteDance Doubao
	{"doubao", 262144},

	// Baidu / Tencent
	{"ernie", 131072},
	{"hunyuan", 131072},

	// MiniMax
	{"minimax", 204800},
	{"minimax-m3", 1000000},

	// Mistral
	{"mistral-large", 262144},
	{"mistral-large-4", 1048576},
	{"mistral-small", 262144},
	{"mistral-medium", 262144},
	{"codestral", 262144},
	{"ministral", 131072},

	// Cohere
	{"command-a", 262144},

	// xAI
	{"grok", 500000},
	{"grok-4.3", 1000000},
	{"grok-4.20", 1000000},

	// Meta / Xiaomi / StepFun
	{"llama-4", 1048576},
	{"mimo", 1048576},
	{"step", 1048576},
}

// sizeSuffixPattern matches an explicit context-window token in a model id,
// such as "-128k", "_32k", "-1m", or "-1.5m". The token must be delimited by
// the start/end of the id or by '-'/'_', so version fragments such as
// "gpt-4-1106-preview" or parameter counts such as "qwen3-coder-30b-a3b" are
// not treated as windows.
var sizeSuffixPattern = regexp.MustCompile(`(?:^|[-_])([0-9]+(?:\.[0-9]+)?)([km])(?:$|[-_])`)

// maxSizeSuffixWindow rejects absurd suffix values so a malformed model id
// cannot overflow the token budget.
const maxSizeSuffixWindow = 100_000_000

// BuiltinContextWindow returns a known context window for model, or 0 when no
// mainstream family matches. It is case-insensitive and ignores a provider
// prefix and a ":variant" suffix.
func BuiltinContextWindow(model string) int {
	name := normalizeModelID(model)
	if name == "" {
		return 0
	}
	if window, ok := windowFromSizeSuffix(name); ok {
		return window
	}
	bestWindow, bestLen := 0, 0
	for _, entry := range modelContextWindow {
		if len(entry.prefix) > bestLen && strings.HasPrefix(name, entry.prefix) {
			bestWindow, bestLen = entry.window, len(entry.prefix)
		}
	}
	return bestWindow
}

// normalizeModelID lowercases the model id, keeps the last path segment, and
// drops a ":free"/":nitro"-style variant suffix.
func normalizeModelID(model string) string {
	name := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.Index(name, ":"); i >= 0 {
		name = name[:i]
	}
	return name
}

// windowFromSizeSuffix reads a window explicitly encoded in the model id. It
// uses decimal units (k=1000, m=1_000_000) so the result is conservative for
// providers that mean the binary 128K/256K/1M sizes.
func windowFromSizeSuffix(name string) (int, bool) {
	match := sizeSuffixPattern.FindStringSubmatch(strings.ToLower(name))
	if len(match) != 3 {
		return 0, false
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil || value <= 0 {
		return 0, false
	}
	multiplier := 1000.0
	if match[2] == "m" {
		multiplier = 1000000.0
	}
	window := int(math.Round(value * multiplier))
	if window <= 0 || window > maxSizeSuffixWindow {
		return 0, false
	}
	return window, true
}
