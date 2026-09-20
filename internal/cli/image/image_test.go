package image

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestBuildImageCurl(t *testing.T) {
	req := &types.GenerateRequest{
		Model:  "gpt-image-2-official",
		Prompt: "test",
	}
	p := &provider.EffectiveProvider{
		BaseURL:      "https://api.apimart.ai/v1",
		APIKey:       "test-key",
		ProviderType: provider.APIMart,
	}
	curl := buildImageCurl(req, p)
	if curl == "" {
		t.Fatal("buildImageCurl() returned empty string")
	}
	if !strings.Contains(curl, "...-key") {
		t.Error("curl should contain masked API key")
	}
	if !strings.Contains(curl, "gpt-image-2-official") {
		t.Error("curl should contain model name")
	}
}

func TestBuildImageCurlEdits(t *testing.T) {
	req := &types.GenerateRequest{
		Model:     "my-model",
		Prompt:    "make it blue",
		Size:      "1024x768",
		ImageURLs: []string{"photo.png"},
	}
	zeekai := func(base string) *provider.EffectiveProvider {
		return &provider.EffectiveProvider{BaseURL: base, APIKey: "test-key", ProviderType: provider.Zeekai}
	}

	t.Run("bare host gains v1", func(t *testing.T) {
		curl := buildImageCurl(req, zeekai("https://api.zeekai.cc"))
		if !strings.Contains(curl, "https://api.zeekai.cc/v1/images/edits") {
			t.Errorf("url should end with /v1/images/edits, got:\n%s", curl)
		}
		if !strings.Contains(curl, `"images":[{"image_url":"photo.png"}]`) {
			t.Errorf("body should embed images[].image_url, got:\n%s", curl)
		}
		if !strings.Contains(curl, "# note: local image files are embedded as data: URIs") {
			t.Errorf("edits dry-run should append the data-URI note, got:\n%s", curl)
		}
		for _, absent := range []string{`"n":`, `"quality":`, `"output_format":`} {
			if strings.Contains(curl, absent) {
				t.Errorf("unset optional field %s should be absent, got:\n%s", absent, curl)
			}
		}
	})

	t.Run("versioned base not doubled", func(t *testing.T) {
		curl := buildImageCurl(req, zeekai("https://api.zeekai.cc/v1"))
		if strings.Contains(curl, "/v1/v1/") {
			t.Errorf("version segment should not be doubled, got:\n%s", curl)
		}
		if !strings.Contains(curl, "https://api.zeekai.cc/v1/images/edits") {
			t.Errorf("url should be /v1/images/edits, got:\n%s", curl)
		}
	})
}

func TestBuildImageCurlDefaultMode(t *testing.T) {
	req := &types.GenerateRequest{
		Model:     "gpt-image-2",
		Prompt:    "a cat",
		ImageURLs: []string{"photo.png"},
	}
	p := &provider.EffectiveProvider{
		BaseURL:      "https://api.apimart.ai",
		APIKey:       "test-key",
		ProviderType: provider.APIMart,
	}
	curl := buildImageCurl(req, p)
	if !strings.Contains(curl, "https://api.apimart.ai/v1/images/generations") {
		t.Errorf("bare host should be normalized to /v1/images/generations, got:\n%s", curl)
	}
	if strings.Contains(curl, "/images/edits") {
		t.Errorf("default mode should not use /images/edits, got:\n%s", curl)
	}
	if !strings.Contains(curl, `"image_urls":["photo.png"]`) {
		t.Errorf("default body should use top-level image_urls, got:\n%s", curl)
	}
}

func TestBuildImageCurlLocalFile(t *testing.T) {
	path := writeLocalPNG(t, t.TempDir())

	t.Run("apimart real file renders upload curl and placeholder", func(t *testing.T) {
		req := &types.GenerateRequest{Model: "gpt-image-2", Prompt: "a cat", ImageURLs: []string{path}}
		p := &provider.EffectiveProvider{BaseURL: "https://api.apimart.ai", APIKey: "test-key", ProviderType: provider.APIMart}
		curl := buildImageCurl(req, p)
		if !strings.Contains(curl, "/uploads/images") {
			t.Errorf("expected an upload curl, got:\n%s", curl)
		}
		if !strings.Contains(curl, "<UPLOAD_URL_0>") {
			t.Errorf("expected the upload placeholder, got:\n%s", curl)
		}
		if strings.Contains(curl, `["`+path+`"]`) {
			t.Errorf("local path should be replaced in the request body, got:\n%s", curl)
		}
	})

	t.Run("default sync embeds a data uri", func(t *testing.T) {
		req := &types.GenerateRequest{Model: "gpt-image-2", Prompt: "a cat", ImageURLs: []string{path}}
		p := &provider.EffectiveProvider{BaseURL: "https://api.openai.com", APIKey: "test-key", ProviderType: provider.OpenAI}
		curl := buildImageCurl(req, p)
		if !strings.Contains(curl, `"image_urls":["data:image/png;base64,`) {
			t.Errorf("expected a data: URI body, got:\n%s", curl)
		}
		if strings.Contains(curl, path) {
			t.Errorf("local path should be encoded, got:\n%s", curl)
		}
	})
}

func TestBuildImageCurlVerbatimPerProvider(t *testing.T) {
	const raw = `{"model":"m","prompt":"p","loras":"u/r","seed":1,"custom_x":9}`
	plain := &types.GenerateRequest{RawJSON: []byte(raw)}
	withImage := &types.GenerateRequest{RawJSON: []byte(raw), ImageURLs: []string{"photo.png"}}

	tests := []struct {
		name    string
		p       *provider.EffectiveProvider
		req     *types.GenerateRequest
		wantURL string
	}{
		{
			"openrouter dedicated image api",
			&provider.EffectiveProvider{BaseURL: "https://openrouter.ai/api/v1", APIKey: "k", ProviderType: provider.OpenRouter},
			plain,
			"https://openrouter.ai/api/v1/images",
		},
		{
			"gemini interactions api",
			&provider.EffectiveProvider{BaseURL: "https://generativelanguage.googleapis.com", APIKey: "k", ProviderType: provider.Gemini},
			plain,
			"https://generativelanguage.googleapis.com/v1/interactions",
		},
		{
			"ollama native generate api",
			&provider.EffectiveProvider{BaseURL: "http://localhost:11434/v1", Type: types.ProviderOllama},
			plain,
			"http://localhost:11434/api/generate",
		},
		{
			"zeekai image input uses images/edits",
			&provider.EffectiveProvider{BaseURL: "https://api.zeekai.cc", APIKey: "k", ProviderType: provider.Zeekai},
			withImage,
			"https://api.zeekai.cc/v1/images/edits",
		},
		{
			"openai-compatible keeps images/generations",
			&provider.EffectiveProvider{BaseURL: "https://api.openai.com", APIKey: "k", ProviderType: provider.OpenAI},
			plain,
			"https://api.openai.com/v1/images/generations",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			curl := buildImageCurl(tc.req, tc.p)
			if !strings.Contains(curl, tc.wantURL) {
				t.Errorf("curl should target %s, got:\n%s", tc.wantURL, curl)
			}
			if !strings.Contains(curl, raw) {
				t.Errorf("curl should forward the --json body verbatim, got:\n%s", curl)
			}
		})
	}
}

func TestBuildImageCurlFlagPathUnchanged(t *testing.T) {
	req := &types.GenerateRequest{Model: "m", Prompt: "p"}
	p := &provider.EffectiveProvider{
		BaseURL:      "https://openrouter.ai/api/v1",
		APIKey:       "k",
		ProviderType: provider.OpenRouter,
	}
	curl := buildImageCurl(req, p)
	for _, want := range []string{"https://openrouter.ai/api/v1/images", `"model":"m"`, `"prompt":"p"`} {
		if !strings.Contains(curl, want) {
			t.Errorf("curl should contain %s, got:\n%s", want, curl)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1023, "1023B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1024 * 1024, "1.0MB"},
		{1024 * 1024 * 3 / 2, "1.5MB"},
	}
	for _, tc := range tests {
		if got := formatBytes(tc.in); got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatParams(t *testing.T) {
	tests := []struct {
		format  string
		quality int
		want    string
	}{
		{"", 0, ""},
		{"jpg", 0, " [jpg]"},
		{"jpg", 85, " [jpg q85]"},
		{"webp", -1, " [webp]"},
	}
	for _, tc := range tests {
		if got := formatParams(tc.format, tc.quality); got != tc.want {
			t.Errorf("formatParams(%q,%d) = %q, want %q", tc.format, tc.quality, got, tc.want)
		}
	}
}
