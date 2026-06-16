package types

import (
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
