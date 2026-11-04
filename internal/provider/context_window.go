package provider

import "strings"

// modelContextWindow is a conservative built-in table for common model
// families. It is only a fallback: an explicit --context-size, config
// context_size, or a provider/model override always wins. Unknown models return
// 0, which disables auto-compaction with a warning instead of guessing.
//
// Values are the documented input context windows for the family. Model ids
// frequently carry date or variant suffixes, so matching is prefix-based after
// stripping a provider prefix such as "anthropic/".
var modelContextWindow = []struct {
	prefix string
	window int
}{
	// OpenAI
	{"gpt-5", 400000},
	{"gpt-4.1", 1047576},
	{"gpt-4o", 128000},
	{"gpt-4-turbo", 128000},
	{"gpt-4-32k", 32768},
	{"gpt-4", 8192},
	{"gpt-3.5-turbo", 16385},
	{"o1", 200000},
	{"o3", 200000},
	{"o4", 200000},
	// Anthropic
	{"claude", 200000},
	// Google
	{"gemini-1.5", 1048576},
	{"gemini-2", 1048576},
	{"gemini", 1048576},
	// DeepSeek
	{"deepseek", 128000},
	// Qwen / Alibaba
	{"qwen", 131072},
	{"qwq", 131072},
	// Zhipu
	{"glm-4", 128000},
	// Moonshot / Kimi
	{"kimi", 128000},
	{"moonshot", 128000},
	// Xiaomi MiMo
	{"mimo", 128000},
	// ByteDance Doubao
	{"doubao", 256000},
	// Meta Llama
	{"llama-3", 128000},
	{"llama-4", 128000},
	// Mistral
	{"mistral-large", 128000},
	{"mistral-small", 128000},
	// Cohere
	{"command-r", 128000},
	// Baidu
	{"ernie", 128000},
	// Tencent
	{"hunyuan", 128000},
	// MiniMax
	{"minimax", 204800},
}

// BuiltinContextWindow returns a known context window for model, or 0 when the
// family is unknown. It is case-insensitive and ignores a provider prefix.
func BuiltinContextWindow(model string) int {
	name := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if name == "" {
		return 0
	}
	for _, entry := range modelContextWindow {
		if strings.HasPrefix(name, entry.prefix) {
			return entry.window
		}
	}
	return 0
}
