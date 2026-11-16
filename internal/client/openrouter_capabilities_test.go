package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func ptrInt(v int) *int { return &v }

func TestFilterOpenRouterImageRequest(t *testing.T) {
	onlyN := map[string]types.OpenRouterParamDescriptor{
		"n": {Type: "range", Min: ptrInt(1), Max: ptrInt(1)},
	}
	aspectOnly := map[string]types.OpenRouterParamDescriptor{
		"aspect_ratio": {Type: "enum", Values: []string{"1:1", "16:9"}},
	}

	t.Run("model declaring only n drops every size-like field", func(t *testing.T) {
		req := &types.GenerateRequest{
			Model:             "tencent/hy-world-2.1-panorama",
			Prompt:            "p",
			Size:              "1:1",
			Quality:           "auto",
			OutputFormat:      "png",
			Background:        "auto",
			Resolution:        "1k",
			Complexity:        "high",
			N:                 ptrInt(1),
			OutputCompression: ptrInt(80),
		}
		got, err := FilterOpenRouterImageRequest(req, onlyN)
		if err != nil {
			t.Fatalf("FilterOpenRouterImageRequest() error = %v", err)
		}
		if got == req {
			t.Fatal("filter must return a copy, not the original")
		}
		if got.Model != req.Model || got.Prompt != req.Prompt {
			t.Errorf("model/prompt must survive, got %+v", got)
		}
		for name, value := range map[string]string{
			"size":          got.Size,
			"quality":       got.Quality,
			"output_format": got.OutputFormat,
			"background":    got.Background,
			"resolution":    got.Resolution,
			"complexity":    got.Complexity,
		} {
			if value != "" {
				t.Errorf("%s = %q, want dropped", name, value)
			}
		}
		if got.OutputCompression != nil {
			t.Error("output_compression should be dropped")
		}
		// The original request must stay untouched for callers that reuse it.
		if req.Size != "1:1" || req.Quality != "auto" {
			t.Error("filter mutated the input request")
		}
	})

	t.Run("ratio-shaped size becomes aspect_ratio when only that is declared", func(t *testing.T) {
		req := &types.GenerateRequest{Model: "m", Prompt: "p", Size: "16:9", Quality: "high"}
		got, err := FilterOpenRouterImageRequest(req, aspectOnly)
		if err != nil {
			t.Fatalf("FilterOpenRouterImageRequest() error = %v", err)
		}
		if got.Size != "" {
			t.Errorf("size = %q, want dropped", got.Size)
		}
		if got.Ratio != "16:9" {
			t.Errorf("ratio = %q, want 16:9", got.Ratio)
		}
		if got.Quality != "" {
			t.Errorf("quality = %q, want dropped", got.Quality)
		}
	})

	t.Run("tier size is dropped when the model has no size parameter", func(t *testing.T) {
		req := &types.GenerateRequest{Model: "m", Prompt: "p", Size: "2K"}
		got, err := FilterOpenRouterImageRequest(req, aspectOnly)
		if err != nil {
			t.Fatalf("FilterOpenRouterImageRequest() error = %v", err)
		}
		if got.Size != "" || got.Ratio != "" {
			t.Errorf("size/ratio = %q/%q, want both dropped", got.Size, got.Ratio)
		}
	})

	t.Run("tier size maps to resolution when the model declares only resolution", func(t *testing.T) {
		resolutionOnly := map[string]types.OpenRouterParamDescriptor{
			"resolution": {Type: "enum", Values: []string{"1K", "2K", "4K"}},
		}
		req := &types.GenerateRequest{Model: "m", Prompt: "p", Size: "2K", Ratio: "16:9"}
		got, err := FilterOpenRouterImageRequest(req, resolutionOnly)
		if err != nil {
			t.Fatalf("FilterOpenRouterImageRequest() error = %v", err)
		}
		if got.Size != "" || got.Ratio != "" {
			t.Errorf("size/ratio = %q/%q, want both dropped", got.Size, got.Ratio)
		}
		if got.Resolution != "2K" {
			t.Errorf("resolution = %q, want 2K", got.Resolution)
		}
	})

	t.Run("two-knob model keeps both dimensions", func(t *testing.T) {
		params := map[string]types.OpenRouterParamDescriptor{
			"aspect_ratio": {Type: "enum", Values: []string{"1:1", "16:9"}},
			"resolution":   {Type: "enum", Values: []string{"1K", "2K", "4K"}},
		}
		req := &types.GenerateRequest{Model: "m", Prompt: "p", Size: "2K", Ratio: "16:9"}
		got, err := FilterOpenRouterImageRequest(req, params)
		if err != nil {
			t.Fatalf("FilterOpenRouterImageRequest() error = %v", err)
		}
		if got.Size != "" || got.Ratio != "16:9" || got.Resolution != "2K" {
			t.Errorf("size/ratio/resolution = %q/%q/%q, want empty/16:9/2K", got.Size, got.Ratio, got.Resolution)
		}
	})

	t.Run("explicit ratio is dropped when only size is declared", func(t *testing.T) {
		params := map[string]types.OpenRouterParamDescriptor{"size": {Type: "boolean"}}
		req := &types.GenerateRequest{Model: "m", Prompt: "p", Size: "2K", Ratio: "16:9"}
		got, err := FilterOpenRouterImageRequest(req, params)
		if err != nil {
			t.Fatalf("FilterOpenRouterImageRequest() error = %v", err)
		}
		if got.Size != "2K" || got.Ratio != "" {
			t.Errorf("size/ratio = %q/%q, want 2K/empty", got.Size, got.Ratio)
		}
	})

	t.Run("reference images on a text-only model error", func(t *testing.T) {
		req := &types.GenerateRequest{Model: "m", Prompt: "p", ImageURLs: []string{"https://example.com/a.png"}}
		if _, err := FilterOpenRouterImageRequest(req, onlyN); err == nil || !strings.Contains(err.Error(), "does not accept reference images") {
			t.Fatalf("error = %v, want a reference-image rejection", err)
		}
	})

	t.Run("n greater than one on a model without n errors", func(t *testing.T) {
		req := &types.GenerateRequest{Model: "m", Prompt: "p", N: ptrInt(4)}
		if _, err := FilterOpenRouterImageRequest(req, aspectOnly); err == nil || !strings.Contains(err.Error(), "does not support n > 1") {
			t.Fatalf("error = %v, want an n>1 rejection", err)
		}
	})

	t.Run("verbatim json body is preserved", func(t *testing.T) {
		req := &types.GenerateRequest{RawJSON: []byte(`{"model":"m","prompt":"p","size":"1:1"}`)}
		got, err := FilterOpenRouterImageRequest(req, onlyN)
		if err != nil {
			t.Fatalf("FilterOpenRouterImageRequest() error = %v", err)
		}
		if got != req {
			t.Error("verbatim --json body must not be rewritten")
		}
	})
}

func TestOpenRouterImageParamsCachesDiscovery(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/images/models" {
			t.Errorf("path = %q, want /v1/images/models", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"m","supported_parameters":{"n":{"type":"range","min":1,"max":1}}}]}`)
	}))
	defer srv.Close()

	params, found := OpenRouterImageParams(srv.URL, "m")
	if !found || len(params) != 1 {
		t.Fatalf("params = %v, found = %v, want the n descriptor", params, found)
	}
	OpenRouterImageParams(srv.URL, "m")
	if got := calls.Load(); got != 1 {
		t.Errorf("discovery calls = %d, want 1 (second lookup must hit the cache)", got)
	}

	// An unknown model is cached as a miss too, so repeated failures do not
	// hammer the discovery endpoint.
	if _, found := OpenRouterImageParams(srv.URL, "missing"); found {
		t.Error("unknown model reported as found")
	}
	OpenRouterImageParams(srv.URL, "missing")
	if got := calls.Load(); got != 2 {
		t.Errorf("discovery calls = %d, want 2 (miss cached)", got)
	}
}

func TestCachedOpenRouterImageParamsIsOffline(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"m","supported_parameters":{"size":{"type":"boolean"}}}]}`)
	}))
	defer srv.Close()

	if _, ok := CachedOpenRouterImageParams(srv.URL, "m"); ok {
		t.Fatal("cold cache reported a hit")
	}
	if calls.Load() != 0 {
		t.Fatal("CachedOpenRouterImageParams performed network I/O")
	}
	if _, ok := OpenRouterImageParams(srv.URL, "m"); !ok {
		t.Fatal("warm-up lookup failed")
	}
	if _, ok := CachedOpenRouterImageParams(srv.URL, "m"); !ok {
		t.Fatal("warm cache reported a miss")
	}
}
