package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/service"
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
	shared.APIKey = "test-key"
	shared.APIBase = "https://api.apimart.ai"
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
	shared.APIKey = "test-key"
	shared.APIBase = "https://api.apimart.ai"
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

// --- Agent Tool Tests ---

func TestToURLs_empty(t *testing.T) {
	got := toURLs("")
	if got != nil {
		t.Errorf("toURLs('') = %v, want nil", got)
	}
}

func TestToURLs_single(t *testing.T) {
	got := toURLs("https://example.com/img.png")
	if len(got) != 1 || got[0] != "https://example.com/img.png" {
		t.Errorf("toURLs() = %v, want [https://example.com/img.png]", got)
	}
}

func TestExecuteToolCall_unknown(t *testing.T) {
	tc := types.ToolCall{
		ID:   "call_1",
		Type: "function",
		Function: types.ToolCallFunction{
			Name:      "nonexistent_tool",
			Arguments: "{}",
		},
	}
	got := executeToolCall(nil, tc)
	if !strings.Contains(got, "unknown tool") {
		t.Errorf("executeToolCall(unknown) = %q, want 'unknown tool'", got)
	}
}

func TestExecuteToolCall_invalidJSON(t *testing.T) {
	tools := []string{"generate_image", "generate_video", "midjourney_imagine",
		"midjourney_describe", "midjourney_reroll", "midjourney_video",
		"ideas", "balance", "task"}
	for _, name := range tools {
		tc := types.ToolCall{
			ID:   "call_1",
			Type: "function",
			Function: types.ToolCallFunction{
				Name:      name,
				Arguments: "{bad json}",
			},
		}
		got := executeToolCall(nil, tc)
		if !strings.Contains(got, "invalid arguments") {
			t.Errorf("executeToolCall(%s, bad json) = %q, want 'invalid arguments'", name, got)
		}
	}
}

func TestBuildAgentTools_allAllowed(t *testing.T) {
	cfg := &types.ChatDefaults{
		Tools: []string{"*"},
	}
	tools := buildAgentTools(cfg)
	if len(tools) == 0 {
		t.Error("buildAgentTools([\"*\"]) returned empty")
	}
}

func TestBuildAgentTools_disabledAll(t *testing.T) {
	cfg := &types.ChatDefaults{
		DisableTools: []string{"*"},
	}
	tools := buildAgentTools(cfg)
	if len(tools) != 0 {
		t.Errorf("buildAgentTools(disable *) = %d tools, want 0", len(tools))
	}
}

func TestBuildAgentTools_filterImageOnly(t *testing.T) {
	cfg := &types.ChatDefaults{
		Tools: []string{"generate_image"},
	}
	tools := buildAgentTools(cfg)
	if len(tools) != 1 || tools[0].Function.Name != "generate_image" {
		t.Errorf("buildAgentTools filter = got %d tools", len(tools))
	}
}

func TestSummarizeToolResult_truncated(t *testing.T) {
	got := summarizeToolResult("test", "This is a very long result that should definitely be truncated because it exceeds eighty characters in total length")
	if len(got) > 83 {
		t.Errorf("summarizeToolResult() = %q (len=%d), want <= 83", got, len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("summarizeToolResult() = %q, should end with '...'", got)
	}
}

func TestSummarizeToolResult_short(t *testing.T) {
	got := summarizeToolResult("test", "short result")
	if got != "short result" {
		t.Errorf("summarizeToolResult() = %q, want 'short result'", got)
	}
}

func TestSummarizeToolResult_firstLine(t *testing.T) {
	got := summarizeToolResult("test", "first line\nsecond line")
	if got != "first line" {
		t.Errorf("summarizeToolResult() = %q, want 'first line'", got)
	}
}

func TestSummarizeToolResult_successPrefix(t *testing.T) {
	got := summarizeToolResult("generate_image", "Successfully generated image: file.png")
	if got != "Successfully generated image: file.png" {
		t.Errorf("summarizeToolResult() = %q, want full success message", got)
	}
}

func TestSummarizeToolResult_errorPrefix(t *testing.T) {
	got := summarizeToolResult("test", "Error: something went wrong")
	if got != "Error: something went wrong" {
		t.Errorf("summarizeToolResult() = %q, want full error message", got)
	}
}

func TestSummarizeToolResult_empty(t *testing.T) {
	got := summarizeToolResult("test", "")
	if got != "" {
		t.Errorf("summarizeToolResult() = %q, want empty", got)
	}
}

func TestSummarizeToolResult_justNewline(t *testing.T) {
	got := summarizeToolResult("test", "\n")
	if got != "" {
		t.Errorf("summarizeToolResult() = %q, want empty", got)
	}
}

func TestModelFilename_vitBase(t *testing.T) {
	got := modelFilename("vit-base")
	if got != "model-vit-base.onnx" {
		t.Errorf("modelFilename('vit-base') = %q, want 'model-vit-base.onnx'", got)
	}
}

func TestModelFilename_distilledVit(t *testing.T) {
	got := modelFilename("distilled-vit")
	if got != "model-distilled-vit.onnx" {
		t.Errorf("modelFilename('distilled-vit') = %q, want 'model-distilled-vit.onnx'", got)
	}
}

func TestModelFilename_unknown(t *testing.T) {
	got := modelFilename("nonexistent")
	if got != "model-vit-base.onnx" {
		t.Errorf("modelFilename('nonexistent') = %q, want default 'model-vit-base.onnx'", got)
	}
}

func TestModelSizeLabel_vitBase(t *testing.T) {
	got := modelSizeLabel("/some/path/model-vit-base.onnx")
	if got != "vit-base" {
		t.Errorf("modelSizeLabel() = %q, want 'vit-base'", got)
	}
}

func TestModelSizeLabel_distilledVit(t *testing.T) {
	got := modelSizeLabel("/some/path/model-distilled-vit.onnx")
	if got != "distilled-vit" {
		t.Errorf("modelSizeLabel() = %q, want 'distilled-vit'", got)
	}
}

func TestModelSizeLabel_unknown(t *testing.T) {
	got := modelSizeLabel("/some/path/other-model.onnx")
	if got != "other-model.onnx" {
		t.Errorf("modelSizeLabel() = %q, want 'other-model.onnx'", got)
	}
}

func TestSafeC2PAVendor_nil(t *testing.T) {
	if got := safeC2PAVendor(nil); got != "" {
		t.Errorf("safeC2PAVendor(nil) = %q, want ''", got)
	}
}

func TestSafeC2PAVendor_notNil(t *testing.T) {
	r := &service.C2PAResult{Vendor: "OpenAI"}
	if got := safeC2PAVendor(r); got != "OpenAI" {
		t.Errorf("safeC2PAVendor() = %q, want 'OpenAI'", got)
	}
}

func TestSafeC2PASource_nil(t *testing.T) {
	if got := safeC2PASource(nil); got != "" {
		t.Errorf("safeC2PASource(nil) = %q, want ''", got)
	}
}

func TestSafeTC260Provider_nil(t *testing.T) {
	if got := safeTC260Provider(nil); got != "" {
		t.Errorf("safeTC260Provider(nil) = %q, want ''", got)
	}
}

func TestSafeSynthIDSource_nil(t *testing.T) {
	if got := safeSynthIDSource(nil); got != "" {
		t.Errorf("safeSynthIDSource(nil) = %q, want ''", got)
	}
}

func TestSafeCameraMake_nil(t *testing.T) {
	if got := safeCameraMake(nil); got != "" {
		t.Errorf("safeCameraMake(nil) = %q, want ''", got)
	}
}

func TestSafeCameraModel_nil(t *testing.T) {
	if got := safeCameraModel(nil); got != "" {
		t.Errorf("safeCameraModel(nil) = %q, want ''", got)
	}
}

func TestSafeCameraMake_notNil(t *testing.T) {
	r := &service.CameraInfo{Make: "Canon", Model: "EOS R5"}
	if got := safeCameraMake(r); got != "Canon" {
		t.Errorf("safeCameraMake() = %q, want 'Canon'", got)
	}
	if got := safeCameraModel(r); got != "EOS R5" {
		t.Errorf("safeCameraModel() = %q, want 'EOS R5'", got)
	}
}

func TestBuildAgentTools_disableVideo(t *testing.T) {
	cfg := &types.ChatDefaults{
		DisableTools: []string{"generate_video"},
	}
	tools := buildAgentTools(cfg)
	for _, t2 := range tools {
		if t2.Function.Name == "generate_video" {
			t.Error("buildAgentTools should have disabled generate_video")
		}
	}
}
