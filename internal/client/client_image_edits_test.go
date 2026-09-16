package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestImageGenerateEdits_requestShape(t *testing.T) {
	var gotPath string
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"created":1,"data":[{"b64_json":"abc"}]}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "")
	resp, err := c.ImageGenerateEdits(&types.GenerateRequest{
		Model:     "my-model",
		Prompt:    "make it blue",
		Size:      "1024x1024",
		ImageURLs: []string{"data:image/png;base64,AAAA"},
	})
	if err != nil {
		t.Fatalf("ImageGenerateEdits() error = %v", err)
	}
	if gotPath != "/v1/images/edits" {
		t.Errorf("path = %q, want /v1/images/edits", gotPath)
	}
	if resp.Created != 1 || len(resp.Data) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}

	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if body["model"] != "my-model" {
		t.Errorf("model = %v, want my-model", body["model"])
	}
	if body["prompt"] != "make it blue" {
		t.Errorf("prompt = %v, want make it blue", body["prompt"])
	}
	if body["size"] != "1024x1024" {
		t.Errorf("size = %v, want 1024x1024", body["size"])
	}
	images, ok := body["images"].([]any)
	if !ok || len(images) != 1 {
		t.Fatalf("images = %v, want array of 1", body["images"])
	}
	first, ok := images[0].(map[string]any)
	if !ok {
		t.Fatalf("images[0] = %v, want object", images[0])
	}
	if first["image_url"] != "data:image/png;base64,AAAA" {
		t.Errorf("images[0].image_url = %v", first["image_url"])
	}
	for _, absent := range []string{"n", "quality", "output_format"} {
		if _, present := body[absent]; present {
			t.Errorf("optional field %q should be absent when unset", absent)
		}
	}
}

func TestImageGenerateEdits_optionalFields(t *testing.T) {
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"created":1,"data":[]}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "")
	n := 2
	_, err := c.ImageGenerateEdits(&types.GenerateRequest{
		Model:        "m",
		Prompt:       "p",
		Size:         "1:1",
		N:            &n,
		Quality:      "high",
		OutputFormat: "png",
		ImageURLs:    []string{"https://example.com/a.png"},
	})
	if err != nil {
		t.Fatalf("ImageGenerateEdits() error = %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if body["n"] != float64(2) {
		t.Errorf("n = %v, want 2", body["n"])
	}
	if body["quality"] != "high" {
		t.Errorf("quality = %v, want high", body["quality"])
	}
	if body["output_format"] != "png" {
		t.Errorf("output_format = %v, want png", body["output_format"])
	}
}
