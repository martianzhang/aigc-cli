package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/types"
)

func TestExtractExt_hasExtension(t *testing.T) {
	got := extractExt("https://example.com/video.mp4")
	if got != ".mp4" {
		t.Errorf("extractExt() = %q, want %q", got, ".mp4")
	}
}

func TestExtractExt_noExtension(t *testing.T) {
	got := extractExt("https://example.com/video")
	if got != ".mp4" {
		t.Errorf("extractExt() = %q, want %q", got, ".mp4")
	}
}

func TestExtractExt_jpg(t *testing.T) {
	got := extractExt("https://example.com/photo.jpg")
	if got != ".jpg" {
		t.Errorf("extractExt() = %q, want %q", got, ".jpg")
	}
}

func TestExtractExt_withQuery(t *testing.T) {
	got := extractExt("https://example.com/video.mp4?token=abc")
	if got != ".mp4" {
		t.Errorf("extractExt() = %q, want %q", got, ".mp4")
	}
}

func TestIsFile_exists(t *testing.T) {
	tmp, _ := os.CreateTemp("", "testfile")
	tmp.Close()
	defer os.Remove(tmp.Name())

	if !isFile(tmp.Name()) {
		t.Errorf("isFile(%q) should be true", tmp.Name())
	}
}

func TestIsFile_notExists(t *testing.T) {
	if isFile("/tmp/nonexistent_file_xyz") {
		t.Error("isFile() should be false for nonexistent file")
	}
}

func TestIsFile_directory(t *testing.T) {
	dir, _ := os.MkdirTemp("", "testdir")
	defer os.Remove(dir)

	if isFile(dir) {
		t.Error("isFile() should be false for directory")
	}
}

func TestReadInput_string(t *testing.T) {
	got, err := readInput("hello world")
	if err != nil {
		t.Fatalf("readInput() error = %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("readInput() = %q, want %q", string(got), "hello world")
	}
}

func TestReadInput_file(t *testing.T) {
	tmp, _ := os.CreateTemp("", "testinput")
	tmp.WriteString("file content")
	tmp.Close()
	defer os.Remove(tmp.Name())

	got, err := readInput(tmp.Name())
	if err != nil {
		t.Fatalf("readInput() error = %v", err)
	}
	if string(got) != "file content" {
		t.Errorf("readInput() = %q, want %q", string(got), "file content")
	}
}

func TestSetIntFlag_changed(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Int("test-flag", 0, "")
	cmd.Flags().Set("test-flag", "42")

	var target *int
	setIntFlag(cmd, "test-flag", &target, 42)
	if target == nil || *target != 42 {
		t.Error("setIntFlag should set target when flag is changed")
	}
}

func TestSetIntFlag_notChanged(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Int("test-flag", 0, "")

	var target *int
	setIntFlag(cmd, "test-flag", &target, 42)
	if target != nil {
		t.Error("setIntFlag should not set target when flag is not changed")
	}
}

func TestSetBoolFlag_changed(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("test-flag", false, "")
	cmd.Flags().Set("test-flag", "true")

	var target *bool
	setBoolFlag(cmd, "test-flag", &target, true)
	if target == nil || *target != true {
		t.Error("setBoolFlag should set target when flag is changed")
	}
}

func TestSetBoolFlag_notChanged(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("test-flag", false, "")

	var target *bool
	setBoolFlag(cmd, "test-flag", &target, true)
	if target != nil {
		t.Error("setBoolFlag should not set target when flag is not changed")
	}
}

func TestSetFloatFlag_changed(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Float64("test-flag", 0, "")
	cmd.Flags().Set("test-flag", "0.7")

	var target *float64
	setFloatFlag(cmd, "test-flag", &target, 0.7)
	if target == nil || *target != 0.7 {
		t.Error("setFloatFlag should set target when flag is changed")
	}
}

func TestSetFloatFlag_notChanged(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Float64("test-flag", 0, "")

	var target *float64
	setFloatFlag(cmd, "test-flag", &target, 0.7)
	if target != nil {
		t.Error("setFloatFlag should not set target when flag is not changed")
	}
}

func TestBuildImageCurl(t *testing.T) {
	apiKey = "test-key"
	apiBase = "https://api.apimart.ai"
	req := &types.GenerateRequest{
		Model:  "gpt-image-2-official",
		Prompt: "test",
	}
	curl := buildImageCurl(req)
	if curl == "" {
		t.Fatal("buildImageCurl() returned empty string")
	}
	if !strings.Contains(curl, "test-key") {
		t.Error("curl should contain API key")
	}
	if !strings.Contains(curl, "gpt-image-2-official") {
		t.Error("curl should contain model name")
	}
}

func TestBuildVideoCurl(t *testing.T) {
	apiKey = "test-key"
	apiBase = "https://api.apimart.ai"
	req := &types.VideoGenerateRequest{
		Model:  "doubao-seedance-2.0",
		Prompt: "test video",
	}
	curl := buildVideoCurl(req)
	if curl == "" {
		t.Fatal("buildVideoCurl() returned empty string")
	}
	if !strings.Contains(curl, "test-key") {
		t.Error("curl should contain API key")
	}
	if !strings.Contains(curl, "doubao-seedance-2.0") {
		t.Error("curl should contain model name")
	}
}
