package provider

import (
	"net/http"
	"testing"
)

func TestSetAttribution_OpenRouter(t *testing.T) {
	t.Setenv("OPENAI_REFERER", "")
	t.Setenv("OPENAI_APP_TITLE", "")

	req, _ := http.NewRequest(http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", nil)
	SetAttribution(req, "https://openrouter.ai/api/v1")

	if got := req.Header.Get(HeaderReferer); got != DefaultReferer {
		t.Errorf("%s = %q, want %q", HeaderReferer, got, DefaultReferer)
	}
	if got := req.Header.Get(HeaderTitle); got != DefaultTitle {
		t.Errorf("%s = %q, want %q", HeaderTitle, got, DefaultTitle)
	}
	if got := req.Header.Get(HeaderCategories); got != DefaultCategories {
		t.Errorf("%s = %q, want %q", HeaderCategories, got, DefaultCategories)
	}
	if req.Header.Get("User-Agent") == "" {
		t.Error("User-Agent should be set")
	}
}

func TestSetAttribution_NonOpenRouter(t *testing.T) {
	t.Setenv("OPENAI_REFERER", "")
	t.Setenv("OPENAI_APP_TITLE", "")

	req, _ := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", nil)
	SetAttribution(req, "https://api.openai.com/v1")

	if got := req.Header.Get(HeaderReferer); got != "" {
		t.Errorf("%s = %q, want empty for a non-OpenRouter base", HeaderReferer, got)
	}
	if got := req.Header.Get(HeaderTitle); got != "" {
		t.Errorf("%s = %q, want empty for a non-OpenRouter base", HeaderTitle, got)
	}
	if got := req.Header.Get(HeaderCategories); got != "" {
		t.Errorf("%s = %q, want empty for a non-OpenRouter base", HeaderCategories, got)
	}
	if req.Header.Get("User-Agent") == "" {
		t.Error("User-Agent should still be set")
	}
}

func TestSetAttribution_EnvOverride(t *testing.T) {
	t.Setenv("OPENAI_REFERER", "https://myapp.com")
	t.Setenv("OPENAI_APP_TITLE", "MyApp")

	req, _ := http.NewRequest(http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", nil)
	SetAttribution(req, "https://openrouter.ai/api/v1")

	if got := req.Header.Get(HeaderReferer); got != "https://myapp.com" {
		t.Errorf("%s = %q, want the env override", HeaderReferer, got)
	}
	if got := req.Header.Get(HeaderTitle); got != "MyApp" {
		t.Errorf("%s = %q, want the env override", HeaderTitle, got)
	}
}
