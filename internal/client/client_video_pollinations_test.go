package client

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestPollinationsVideoURL(t *testing.T) {
	c := NewWithProvider("sk_test", "https://gen.pollinations.ai", "", types.ProviderOpenAI)
	if c.BaseURL() != "https://gen.pollinations.ai/v1" {
		t.Fatalf("normalized base URL = %q, want .../v1", c.BaseURL())
	}

	duration := 4
	seed := 7
	req := &types.VideoGenerateRequest{
		Model:      "community/NamanSoni78/Seedance-2.5",
		Prompt:     "a cat walking",
		Duration:   &duration,
		Resolution: "720p",
		Seed:       &seed,
	}

	got := c.PollinationsVideoURL(req)

	if strings.Contains(got, "/v1/video/") {
		t.Errorf("URL must not contain the /v1 prefix: %s", got)
	}
	if !strings.HasPrefix(got, "https://gen.pollinations.ai/video/") {
		t.Errorf("URL should target the root /video endpoint: %s", got)
	}
	if !strings.Contains(got, "a%20cat%20walking") {
		t.Errorf("prompt should be path-escaped: %s", got)
	}
	for _, want := range []string{
		"model=community%2FNamanSoni78%2FSeedance-2.5",
		"duration=4",
		"seed=7",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("URL missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "resolution=") {
		t.Errorf("resolution must not be forwarded (the endpoint rejects it): %s", got)
	}
}

func TestPollinationsVideoURL_StartFrame(t *testing.T) {
	c := NewWithProvider("sk_test", "https://gen.pollinations.ai", "", types.ProviderOpenAI)

	lastFrame := &types.VideoGenerateRequest{
		Model:     "m",
		Prompt:    "p",
		ImageURLs: []string{"https://example.com/a.jpg", "https://example.com/b.jpg"},
	}
	if got := c.PollinationsVideoURL(lastFrame); !strings.Contains(got, "image=https%3A%2F%2Fexample.com%2Fa.jpg") {
		t.Errorf("first image_url should become the start frame: %s", got)
	}

	withRole := &types.VideoGenerateRequest{
		Model:  "m",
		Prompt: "p",
		ImageWithRoles: []types.ImageWithRole{
			{URL: "https://example.com/first.jpg", Role: "first_frame"},
		},
	}
	if got := c.PollinationsVideoURL(withRole); !strings.Contains(got, "image=https%3A%2F%2Fexample.com%2Ffirst.jpg") {
		t.Errorf("first_frame role should become the start frame: %s", got)
	}
}

func TestPollinationsVideoURL_MinimalRequest(t *testing.T) {
	c := NewWithProvider("", "https://gen.pollinations.ai/v1", "", types.ProviderOpenAI)
	got := c.PollinationsVideoURL(&types.VideoGenerateRequest{Model: "flux", Prompt: "hi"})
	if got != "https://gen.pollinations.ai/video/hi?model=flux" {
		t.Errorf("unexpected URL: %s", got)
	}
}
