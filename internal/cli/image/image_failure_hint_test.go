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
			name: "status 400 without size",
			err:  status400,
			req:  &types.GenerateRequest{},
			want: false,
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
		{"gemini", provider.Gemini, "derives aspect_ratio"},
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

	t.Run("lists supported values", func(t *testing.T) {
		got := openRouterSizeHint(p, &types.GenerateRequest{Model: "recraft/recraft-v4.1-flash", Size: "2K"})
		for _, want := range []string{"aspect ratio (--ratio): 1:1 | 16:9 | auto", "resolution tier (--size): 1K | 2K"} {
			if !strings.Contains(got, want) {
				t.Errorf("hint missing %q:\n%s", want, got)
			}
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
}
