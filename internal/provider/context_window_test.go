package provider

import (
	"strings"
	"testing"
)

func TestBuiltinContextWindow(t *testing.T) {
	cases := []struct {
		model string
		want  int
	}{
		// 2026 mainstream families.
		{"gpt-5.6", 1050000},
		{"gpt-5.6-luna", 1050000},
		{"gpt-6", 1050000},
		{"openai/gpt-6.1-sol", 1050000},
		{"anthropic/claude-sonnet-4", 200000},
		{"claude-opus-4-6", 1000000},
		{"claude-sonnet-4-6-2026-01-01", 1000000},
		{"claude-opus-5", 1000000},
		{"gemini-2.5-pro", 1048576},
		{"gemma-4-31b-it", 262144},
		{"deepseek-chat", 128000},
		{"deepseek-v4-pro", 1048576},
		{"qwen-plus", 1000000},
		{"qwen-flash", 1000000},
		{"qwen3-235b-a22b", 131072},
		{"glm-5.2", 1000000},
		{"glm-5.3-flash", 1000000},
		{"kimi-k2.6", 262144},
		{"kimi-k3", 1048576},
		{"doubao-pro", 262144},
		{"doubao-pro-256k", 256000},
		{"ernie-4.5", 131072},
		{"ernie-4.0-8k", 8000},
		{"hunyuan-standard-32k", 32000},
		{"minimax-m3", 1000000},
		{"minimax-m2.5", 204800},
		{"mistral-large-4", 1048576},
		{"mistral-large-latest", 262144},
		{"mistral-small-2603", 262144},
		{"codestral-2508", 262144},
		{"ministral-8b-latest", 131072},
		{"command-a-03-2025", 262144},
		{"grok-4.6", 500000},
		{"grok-4.3", 1000000},
		{"grok-4.20-0309-reasoning", 1000000},
		{"llama-4-scout-17b", 1048576},
		{"mimo-v2.6", 1048576},
		{"step-5-preview", 1048576},

		// Retired or short-prefix models fall through to the caller's fallback.
		{"gpt-4o", 0},
		{"gpt-4o-2024-08-06", 0},
		{"gpt-5", 0},
		{"gpt-5.5", 0},
		{"gpt-4.1", 0},
		{"o3", 0},
		{"o4-mini", 0},
		{"glm-5", 0},
		{"glm-4.7", 0},
		{"llama-3-8b", 0},
		{"unknown-model", 0},
		{"", 0},

		// Explicit window suffixes still work without a table entry.
		{"gpt-4-32k", 32000},
		{"moonshot-v1-8k", 8000},
		{"doubao-pro-4k", 4000},

		// Normalization: case, whitespace, provider prefix, and variant suffix.
		{"OpenAI/GPT-5.6", 1050000},
		{"openai/gpt-5.6:free", 1050000},
		{"  gemini-2.5-pro  ", 1048576},
		{"openrouter/anthropic/claude-opus-4-6", 1000000},

		// Longest prefix wins over the family default.
		{"claude-sonnet-4-6", 1000000},
		{"deepseek-v4-flash", 1048576},
		{"kimi-k2.7-code", 262144},
		{"mistral-large-4", 1048576},
		{"grok-4.3", 1000000},
		{"qwen-plus", 1000000},
		{"minimax-m3.1-flash", 1000000},

		// qwen-max is below 128K and intentionally not listed; the qwen family
		// default applies. Users can override it with context_size if needed.
		{"qwen-max", 131072},
	}
	for _, c := range cases {
		if got := BuiltinContextWindow(c.model); got != c.want {
			t.Errorf("BuiltinContextWindow(%q) = %d, want %d", c.model, got, c.want)
		}
	}
}

func TestModelContextWindowTable(t *testing.T) {
	seen := make(map[string]bool, len(modelContextWindow))
	for _, entry := range modelContextWindow {
		if entry.prefix == "" {
			t.Error("modelContextWindow contains an empty prefix")
			continue
		}
		if entry.prefix != strings.ToLower(entry.prefix) {
			t.Errorf("modelContextWindow prefix %q is not lowercase", entry.prefix)
		}
		if len(entry.prefix) < 4 {
			t.Errorf("modelContextWindow prefix %q is too short and prone to false matches", entry.prefix)
		}
		if entry.window < 128000 {
			t.Errorf("modelContextWindow prefix %q window %d is below the 128K floor", entry.prefix, entry.window)
		}
		if seen[entry.prefix] {
			t.Errorf("modelContextWindow contains duplicate prefix %q", entry.prefix)
		}
		seen[entry.prefix] = true
	}
}

func TestNormalizeModelID(t *testing.T) {
	cases := map[string]string{
		"OpenAI/GPT-5.6":                       "gpt-5.6",
		"openai/gpt-5.6:free":                  "gpt-5.6",
		"  gemini-2.5-pro  ":                   "gemini-2.5-pro",
		"openrouter/anthropic/claude-opus-4-6": "claude-opus-4-6",
		"":                                     "",
	}
	for model, want := range cases {
		if got := normalizeModelID(model); got != want {
			t.Errorf("normalizeModelID(%q) = %q, want %q", model, got, want)
		}
	}
}

func TestWindowFromSizeSuffix(t *testing.T) {
	cases := []struct {
		model string
		want  int
		ok    bool
	}{
		{"model-128k", 128000, true},
		{"model_32k", 32000, true},
		{"model-1m", 1000000, true},
		{"model-1.5m", 1500000, true},
		{"model-256K", 256000, true},
		{"model-128k-2026", 128000, true},
		{"model-2026-128k", 128000, true},
		{"model-10m", 10000000, true},
		{"gpt-4-1106-preview", 0, false},
		{"qwen3-coder-30b-a3b", 0, false},
		{"kimi-k2.6", 0, false},
		{"gpt-4.1", 0, false},
		{"gpt-5.3-codex-spark", 0, false},
		{"model-128kb", 0, false},
		{"model-0k", 0, false},
		{"model-1.5", 0, false},
		{"model-1.5g", 0, false},
		{"model-999999999m", 0, false},
		{"model-", 0, false},
	}
	for _, c := range cases {
		got, ok := windowFromSizeSuffix(c.model)
		if ok != c.ok || got != c.want {
			t.Errorf("windowFromSizeSuffix(%q) = (%d, %t), want (%d, %t)", c.model, got, ok, c.want, c.ok)
		}
	}
}
