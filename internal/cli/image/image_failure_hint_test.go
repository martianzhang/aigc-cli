package image

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestLooksLikeSizeError(t *testing.T) {
	status400 := &client.APIStatusError{StatusCode: http.StatusBadRequest, Body: `{"error":{"message":"bad"}}`}
	tests := []struct {
		name string
		err  error
		req  *types.GenerateRequest
		want bool
	}{
		{
			name: "explicit size keyword",
			err:  errors.New(`API returned status 400: {"error":{"message":"Novita cannot send size \"16:9\""}}`),
			req:  &types.GenerateRequest{Size: "16:9"},
			want: true,
		},
		{
			name: "aspect keyword",
			err:  errors.New("unsupported aspect ratio"),
			req:  &types.GenerateRequest{Size: "16:9"},
			want: true,
		},
		{
			name: "chinese size keyword",
			err:  errors.New("不支持的尺寸"),
			req:  &types.GenerateRequest{Size: "16:9"},
			want: true,
		},
		{
			name: "status 400 with size set",
			err:  fmt.Errorf("image generation failed: %w", status400),
			req:  &types.GenerateRequest{Size: "1024x1024"},
			want: true,
		},
		{
			name: "generation in the message is not a size keyword",
			err:  errors.New(`OpenRouter image generation failed: API returned status 403: {"error":{"message":"This model is not available in your region."}}`),
			req:  &types.GenerateRequest{Size: "2K"},
			want: false,
		},
		{
			name: "status 400 without size",
			err:  status400,
			req:  &types.GenerateRequest{},
			want: false,
		},
		{
			name: "content safety error is not a size error",
			err:  errors.New(`image edits (multipart) failed: API returned status 400: {"error":{"message":"The request was rejected by the content safety policy. Please revise the input and try again."}}`),
			req:  &types.GenerateRequest{Size: "1024x768"},
			want: false,
		},
		{
			name: "chinese moderation error is not a size error",
			err:  errors.New("API returned status 400: 请求被内容安全策略拒绝"),
			req:  &types.GenerateRequest{Size: "1024x768"},
			want: false,
		},
		{
			name: "explicit invalid size still hints",
			err:  errors.New(`API returned status 400: {"error":{"message":"Invalid size '16:16'. Expected WIDTHxHEIGHT, for example '1824x1024'."}}`),
			req:  &types.GenerateRequest{Size: "16:16"},
			want: true,
		},
		{
			name: "auth error is ignored",
			err:  errors.New("API returned status 401: unauthorized"),
			req:  &types.GenerateRequest{Size: "1024x1024"},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksLikeSizeError(tc.err, tc.req); got != tc.want {
				t.Errorf("looksLikeSizeError() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStaticSizeHint(t *testing.T) {
	tests := []struct {
		name     string
		provType provider.Type
		want     string
	}{
		{"apimart", provider.APIMart, "--resolution 1k/2k/4k"},
		{"agnes", provider.Agnes, "2K@16:9"},
		{"gemini", provider.Gemini, "aspect_ratio + image_size"},
		{"modelscope", provider.ModelScope, "pixels only"},
		{"openai", provider.OpenAI, "ratio (16:9)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &provider.EffectiveProvider{ProviderType: tc.provType}
			got := staticSizeHint(p)
			if !strings.Contains(got, tc.want) {
				t.Errorf("staticSizeHint() = %q, want substring %q", got, tc.want)
			}
		})
	}
}

func TestOpenRouterSizeHint(t *testing.T) {
	models := types.OpenRouterMediaModelList{Data: []types.OpenRouterMediaModel{
		{
			ID: "recraft/recraft-v4.1-flash",
			SupportedParameters: map[string]types.OpenRouterParamDescriptor{
				"aspect_ratio": {Type: "enum", Values: []string{"1:1", "16:9", "auto"}},
				"resolution":   {Type: "enum", Values: []string{"1K", "2K"}},
			},
		},
		{
			ID: "openai/gpt-image-2",
			SupportedParameters: map[string]types.OpenRouterParamDescriptor{
				"aspect_ratio": {Type: "enum", Values: []string{"1:1", "3:2", "auto"}},
			},
		},
		{
			ID: "vendor/resolution-only",
			SupportedParameters: map[string]types.OpenRouterParamDescriptor{
				"resolution": {Type: "enum", Values: []string{"1K", "2K"}},
			},
		},
		{ID: "inclusionai/ming-image-0.1-design", SupportedParameters: map[string]types.OpenRouterParamDescriptor{}},
	}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/models" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(models)
	}))
	defer srv.Close()

	p := &provider.EffectiveProvider{ProviderType: provider.OpenRouter, BaseURL: srv.URL + "/v1"}

	t.Run("lists supported values and how to combine them", func(t *testing.T) {
		got := openRouterSizeHint(p, &types.GenerateRequest{Model: "recraft/recraft-v4.1-flash", Size: "2K"})
		for _, want := range []string{
			"aspect ratio (--ratio): 1:1 | 16:9 | auto",
			"resolution tier (--size or --resolution): 1K | 2K",
			"combine with --size <tier>@<ratio> (e.g. 2K@16:9)",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("hint missing %q:\n%s", want, got)
			}
		}
	})

	t.Run("ratio-only model is not told to combine", func(t *testing.T) {
		got := openRouterSizeHint(p, &types.GenerateRequest{Model: "openai/gpt-image-2", Size: "2K"})
		if !strings.Contains(got, "aspect ratio (--ratio): 1:1 | 3:2 | auto") {
			t.Errorf("hint should list the supported ratios:\n%s", got)
		}
		if !strings.Contains(got, "declares no resolution tier") {
			t.Errorf("hint should explain the model has no tier:\n%s", got)
		}
		if strings.Contains(got, "combine with") || strings.Contains(got, "resolution tier (--size") {
			t.Errorf("hint must not mention a resolution tier the model lacks:\n%s", got)
		}
	})

	t.Run("tier-only model is told to pass the tier alone", func(t *testing.T) {
		got := openRouterSizeHint(p, &types.GenerateRequest{Model: "vendor/resolution-only", Size: "16:9"})
		if !strings.Contains(got, "declares no aspect ratio") {
			t.Errorf("hint should explain the model has no aspect ratio:\n%s", got)
		}
		if strings.Contains(got, "combine with") {
			t.Errorf("hint must not suggest combining missing fields:\n%s", got)
		}
	})

	t.Run("model without size params", func(t *testing.T) {
		got := openRouterSizeHint(p, &types.GenerateRequest{Model: "inclusionai/ming-image-0.1-design", Size: "16:9"})
		if !strings.Contains(got, "declares no size parameter") {
			t.Errorf("hint should tell the user to drop --size:\n%s", got)
		}
	})

	t.Run("unknown model falls back", func(t *testing.T) {
		got := openRouterSizeHint(p, &types.GenerateRequest{Model: "unknown/model", Size: "16:9"})
		if !strings.Contains(got, "could not read this model's capabilities") {
			t.Errorf("hint should fall back to generic guidance:\n%s", got)
		}
	})
}

func TestImageFailureHintDispatch(t *testing.T) {
	p := &provider.EffectiveProvider{ProviderType: provider.Gemini}
	err := errors.New("API returned status 400: invalid aspect ratio")

	if got := imageFailureHint(nil, &types.GenerateRequest{}, err); got != "" {
		t.Errorf("nil provider should produce no hint, got %q", got)
	}
	if got := imageFailureHint(p, &types.GenerateRequest{}, errors.New("401 unauthorized")); got != "" {
		t.Errorf("unrelated error should produce no hint, got %q", got)
	}
	if got := imageFailureHint(p, &types.GenerateRequest{Size: "16:9"}, err); !strings.Contains(got, "Gemini") {
		t.Errorf("expected Gemini hint, got %q", got)
	}
	safetyErr := errors.New(`API returned status 400: {"error":{"message":"The request was rejected by the content safety policy."}}`)
	if got := imageFailureHint(p, &types.GenerateRequest{Size: "1024x768"}, safetyErr); got != "" {
		t.Errorf("content-safety error must not append a size hint, got %q", got)
	}
}
