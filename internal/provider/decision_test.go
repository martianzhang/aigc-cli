package provider

import "testing"

func TestSystemOneEndpoint(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{"typesafe bare", "https://api.typesafe.ai", "https://api.typesafe.ai/v1/systemone"},
		{"typesafe trailing slash", "https://api.typesafe.ai/", "https://api.typesafe.ai/v1/systemone"},
		{"openrouter api", "https://openrouter.ai/api", "https://openrouter.ai/api/v1/systemone"},
		{"openrouter api v1", "https://openrouter.ai/api/v1", "https://openrouter.ai/api/v1/systemone"},
		{"openrouter api v1 trailing slash", "https://openrouter.ai/api/v1/", "https://openrouter.ai/api/v1/systemone"},
		{"openrouter bare", "https://openrouter.ai", "https://openrouter.ai/api/v1/systemone"},
		{"ollama bare", "http://localhost:11434", "http://localhost:11434/v1/systemone"},
		{"ollama v1", "http://localhost:11434/v1", "http://localhost:11434/v1/systemone"},
		{"llm gateway v1", "https://api.llmgateway.io/v1", "https://api.llmgateway.io/v1/systemone"},
		{"litellm proxy", "http://localhost:4000/typesafe", "http://localhost:4000/typesafe/v1/systemone"},
	}
	for _, tc := range tests {
		if got := SystemOneEndpoint(tc.base); got != tc.want {
			t.Errorf("%s: SystemOneEndpoint(%q) = %q, want %q", tc.name, tc.base, got, tc.want)
		}
	}
}
