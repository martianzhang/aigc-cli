package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMergeIntoImage_nil(t *testing.T) {
	var d *ImageDefaults
	req := &GenerateRequest{Model: "test"}
	d.MergeIntoImage(req)
	if req.Model != "test" {
		t.Error("MergeIntoImage on nil should not change anything")
	}
}

func TestMergeIntoImage_emptyRequest(t *testing.T) {
	d := &ImageDefaults{
		Model:      "gpt-image-2-official",
		Size:       "3:1",
		Resolution: "1k",
		Quality:    "low",
	}
	req := &GenerateRequest{}
	d.MergeIntoImage(req)
	if req.Model != "gpt-image-2-official" {
		t.Errorf("Model = %q, want %q", req.Model, "gpt-image-2-official")
	}
	if req.Size != "3:1" {
		t.Errorf("Size = %q, want %q", req.Size, "3:1")
	}
	if req.Resolution != "1k" {
		t.Errorf("Resolution = %q, want %q", req.Resolution, "1k")
	}
	if req.Quality != "low" {
		t.Errorf("Quality = %q, want %q", req.Quality, "low")
	}
}

func TestMergeIntoImage_requestTakesPrecedence(t *testing.T) {
	d := &ImageDefaults{
		Model:      "default-model",
		Size:       "1:1",
		Resolution: "2k",
	}
	req := &GenerateRequest{
		Model: "my-model",
		Size:  "16:9",
	}
	d.MergeIntoImage(req)
	if req.Model != "my-model" {
		t.Errorf("Request Model should take precedence, got %q", req.Model)
	}
	if req.Size != "16:9" {
		t.Errorf("Request Size should take precedence, got %q", req.Size)
	}
	if req.Resolution != "2k" {
		t.Errorf("Default Resolution should be applied, got %q", req.Resolution)
	}
}

func TestMergeIntoImage_pointerFields(t *testing.T) {
	n := 4
	compression := 85
	d := &ImageDefaults{
		N:                 &n,
		OutputCompression: &compression,
	}
	req := &GenerateRequest{}
	d.MergeIntoImage(req)
	if req.N == nil || *req.N != 4 {
		t.Error("N should be merged from defaults")
	}
	if req.OutputCompression == nil || *req.OutputCompression != 85 {
		t.Error("OutputCompression should be merged from defaults")
	}
}

func TestMergeIntoImage_sliceFields(t *testing.T) {
	d := &ImageDefaults{
		ImageURLs: []string{"https://example.com/img.png"},
	}
	req := &GenerateRequest{}
	d.MergeIntoImage(req)
	if len(req.ImageURLs) != 1 || req.ImageURLs[0] != "https://example.com/img.png" {
		t.Error("ImageURLs should be merged from defaults")
	}
}

func TestMergeIntoImage_requestSliceTakesPrecedence(t *testing.T) {
	d := &ImageDefaults{
		ImageURLs: []string{"https://default.com/img.png"},
	}
	req := &GenerateRequest{
		ImageURLs: []string{"https://request.com/img.png"},
	}
	d.MergeIntoImage(req)
	if len(req.ImageURLs) != 1 || req.ImageURLs[0] != "https://request.com/img.png" {
		t.Error("Request ImageURLs should take precedence")
	}
}

func TestMergeIntoVideo_nil(t *testing.T) {
	var d *VideoDefaults
	req := &VideoGenerateRequest{Model: "test"}
	d.MergeIntoVideo(req)
	if req.Model != "test" {
		t.Error("MergeIntoVideo on nil should not change anything")
	}
}

func TestMergeIntoVideo_emptyRequest(t *testing.T) {
	d := &VideoDefaults{
		Model:      "doubao-seedance-2.0",
		Size:       "16:9",
		Resolution: "480p",
	}
	req := &VideoGenerateRequest{}
	d.MergeIntoVideo(req)
	if req.Model != "doubao-seedance-2.0" {
		t.Errorf("Model = %q, want %q", req.Model, "doubao-seedance-2.0")
	}
	if req.Size != "16:9" {
		t.Errorf("Size = %q, want %q", req.Size, "16:9")
	}
}

func TestMergeIntoVideo_requestTakesPrecedence(t *testing.T) {
	d := &VideoDefaults{
		Model: "default-model",
		Size:  "1:1",
	}
	req := &VideoGenerateRequest{
		Model: "my-model",
	}
	d.MergeIntoVideo(req)
	if req.Model != "my-model" {
		t.Errorf("Request Model should take precedence, got %q", req.Model)
	}
	if req.Size != "1:1" {
		t.Errorf("Default Size should be applied, got %q", req.Size)
	}
}

func TestMergeIntoVideo_duration(t *testing.T) {
	dur := 8
	d := &VideoDefaults{Duration: &dur}
	req := &VideoGenerateRequest{}
	d.MergeIntoVideo(req)
	if req.Duration == nil || *req.Duration != 8 {
		t.Error("Duration should be merged from defaults")
	}
}

func TestMergeIntoVideo_slices(t *testing.T) {
	d := &VideoDefaults{
		ImageURLs: []string{"img.png"},
		VideoURLs: []string{"vid.mp4"},
		AudioURLs: []string{"aud.mp3"},
	}
	req := &VideoGenerateRequest{}
	d.MergeIntoVideo(req)
	if len(req.ImageURLs) != 1 || len(req.VideoURLs) != 1 || len(req.AudioURLs) != 1 {
		t.Error("All slices should be merged from defaults")
	}
}

func TestValidateBackground(t *testing.T) {
	tests := []struct {
		name       string
		background string
		format     string
		wantErr    bool
	}{
		{"transparent+png", "transparent", "png", false},
		{"transparent+webp", "transparent", "webp", false},
		{"transparent+empty (defaults to png)", "transparent", "", false},
		{"transparent+jpeg", "transparent", "jpeg", true},
		{"transparent+avif", "transparent", "avif", true},
		{"transparent+uppercase", "Transparent", "PNG", false},
		{"opaque+jpeg", "opaque", "jpeg", false},
		{"auto+avif", "auto", "avif", false},
		{"empty+png", "", "png", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &GenerateRequest{Background: tt.background, OutputFormat: tt.format}
			err := r.ValidateBackground()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBackground() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAnimation(t *testing.T) {
	intp := func(v int) *int { return &v }
	tests := []struct {
		name      string
		req       GenerateRequest
		wantErr   bool
		errSubstr string
	}{
		{"gif 2.6 flare", GenerateRequest{Model: "gpt-image-2.6-flare", OutputFormat: "gif"}, false, ""},
		{"gif 2.6 prefixed", GenerateRequest{Model: "openai/gpt-image-2.6-sunburst", OutputFormat: "GIF"}, false, ""},
		{"gif 3.0", GenerateRequest{Model: "gpt-image-3", OutputFormat: "gif"}, false, ""},
		{"gif 2.5 rejected", GenerateRequest{Model: "gpt-image-2.5-sunburst", OutputFormat: "gif"}, true, "gpt-image-2.6"},
		{"gif gpt-image-2 rejected", GenerateRequest{Model: "openai/gpt-image-2", OutputFormat: "gif"}, true, "gpt-image-2.6"},
		{"gif unknown model passes", GenerateRequest{Model: "some-custom-gif-model", OutputFormat: "gif"}, false, ""},
		{"gif with compression rejected", GenerateRequest{Model: "gpt-image-2.6-flare", OutputFormat: "gif", OutputCompression: intp(50)}, true, "output_compression"},
		{"frame count with png rejected", GenerateRequest{Model: "gpt-image-2.6-flare", OutputFormat: "png", FrameCount: intp(4)}, true, "frame_count"},
		{"frame duration with png rejected", GenerateRequest{Model: "gpt-image-2.6-flare", OutputFormat: "png", FrameDurationMs: intp(100)}, true, "frame_duration_ms"},
		{"frame count too low", GenerateRequest{Model: "gpt-image-2.6-flare", OutputFormat: "gif", FrameCount: intp(1)}, true, "between 2 and 64"},
		{"frame count too high", GenerateRequest{Model: "gpt-image-2.6-flare", OutputFormat: "gif", FrameCount: intp(65)}, true, "between 2 and 64"},
		{"frame count bounds", GenerateRequest{Model: "gpt-image-2.6-flare", OutputFormat: "gif", FrameCount: intp(64)}, false, ""},
		{"frame duration too low", GenerateRequest{Model: "gpt-image-2.6-flare", OutputFormat: "gif", FrameDurationMs: intp(10)}, true, "between 20 and 2000"},
		{"frame duration too high", GenerateRequest{Model: "gpt-image-2.6-flare", OutputFormat: "gif", FrameDurationMs: intp(2001)}, true, "between 20 and 2000"},
		{"frame duration bounds", GenerateRequest{Model: "gpt-image-2.6-flare", OutputFormat: "gif", FrameDurationMs: intp(2000)}, false, ""},
		{"gif frame params", GenerateRequest{Model: "gpt-image-2.6-flare", OutputFormat: "gif", FrameCount: intp(9), FrameDurationMs: intp(200)}, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.ValidateAnimation()
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateAnimation() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("error = %q, want substring %q", err, tt.errSubstr)
			}
		})
	}
}

func TestGenerateRequestMarshalJSONAnimationFields(t *testing.T) {
	fc, fd := 4, 150
	out, err := json.Marshal(GenerateRequest{
		Model:           "gpt-image-2.6-flare",
		Prompt:          "p",
		OutputFormat:    "gif",
		Complexity:      "high",
		FrameCount:      &fc,
		FrameDurationMs: &fd,
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	for _, want := range []string{`"output_format":"gif"`, `"complexity":"high"`, `"frame_count":4`, `"frame_duration_ms":150`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("marshal = %s, missing %s", out, want)
		}
	}

	empty, err := json.Marshal(GenerateRequest{Model: "m", Prompt: "p"})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(empty), "frame_") || strings.Contains(string(empty), "complexity") {
		t.Errorf("empty animation fields leaked: %s", empty)
	}
}

func TestGenerateRequestMarshalJSONPassthrough(t *testing.T) {
	raw := []byte(`{"model":"m","prompt":"p","loras":{"a":0.7,"b":0.3},"seed":1}`)
	out, err := json.Marshal(GenerateRequest{RawJSON: raw})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if string(out) != string(raw) {
		t.Errorf("Marshal() = %s, want verbatim %s", out, raw)
	}

	typed, err := json.Marshal(GenerateRequest{Model: "m", Prompt: "p"})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !strings.Contains(string(typed), `"model":"m"`) {
		t.Errorf("typed marshal = %s", typed)
	}
	if strings.Contains(string(typed), "RawJSON") {
		t.Errorf("RawJSON leaked into typed marshal: %s", typed)
	}
}
