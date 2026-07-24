package mcp

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// ----- buildAudioDesc -----

func Test_buildAudioDesc_withOpenAI(t *testing.T) {
	d := &types.AudioDefaults{
		SpeakModel:      "tts-1",
		TranscribeModel: "whisper-1",
		Voice:           "alloy",
		Format:          "mp3",
	}
	got := buildAudioDesc(d, "https://api.openai.com/v1")
	if !strings.Contains(got, "OpenAI") {
		t.Errorf("expected description to mention OpenAI, got: %s", got)
	}
	if !strings.Contains(got, "tts-1") {
		t.Errorf("expected description to contain speak_model, got: %s", got)
	}
	if !strings.Contains(got, "alloy") {
		t.Errorf("expected description to contain voice, got: %s", got)
	}
}

func Test_buildAudioDesc_withOpenRouter(t *testing.T) {
	d := &types.AudioDefaults{
		SpeakModel:      "openai/gpt-4o-mini-tts",
		TranscribeModel: "openai/whisper-1",
		Voice:           "nova",
		Format:          "wav",
	}
	got := buildAudioDesc(d, "https://openrouter.ai/api/v1")
	if !strings.Contains(got, "OpenRouter") {
		t.Errorf("expected description to mention OpenRouter, got: %s", got)
	}
	if !strings.Contains(got, "gpt-4o-mini-tts") {
		t.Errorf("expected description to contain speak_model, got: %s", got)
	}
}

func Test_buildAudioDesc_nilDefaults(t *testing.T) {
	got := buildAudioDesc(nil, "https://api.openai.com/v1")
	if !strings.Contains(got, "OpenAI") {
		t.Errorf("expected description to mention OpenAI, got: %s", got)
	}
	if !strings.Contains(got, "Strategy:") {
		t.Errorf("expected description to contain strategy note, got: %s", got)
	}
}

// ----- cmdProvider -----

func Test_cmdProvider_mapHasKey(t *testing.T) {
	expected := &provider.EffectiveProvider{
		APIKey:  "sk-test",
		BaseURL: "https://custom.example.com",
	}
	cfg := &Config{
		APIKey:  "sk-global",
		BaseURL: "https://api.openai.com/v1",
		CmdProviders: map[string]*provider.EffectiveProvider{
			"image": expected,
		},
	}
	got := cfg.cmdProvider("image")
	if got != expected {
		t.Errorf("expected pre-resolved provider, got %+v", got)
	}
}

func Test_cmdProvider_mapMissing(t *testing.T) {
	cfg := &Config{
		APIKey:       "sk-global",
		BaseURL:      "https://api.openai.com/v1",
		Proxy:        "http://proxy.example.com:8080",
		CmdProviders: map[string]*provider.EffectiveProvider{},
	}
	got := cfg.cmdProvider("video")
	if got.APIKey != "sk-global" {
		t.Errorf("expected APIKey=sk-global, got %q", got.APIKey)
	}
	if got.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("expected BaseURL fallback, got %q", got.BaseURL)
	}
	if got.HTTPProxy != "http://proxy.example.com:8080" {
		t.Errorf("expected Proxy fallback, got %q", got.HTTPProxy)
	}
	if got.ProviderType != provider.OpenAI {
		t.Errorf("expected ProviderType=OpenAI, got %v", got.ProviderType)
	}
}

func Test_cmdProvider_nilMap(t *testing.T) {
	cfg := &Config{
		APIKey:  "sk-global",
		BaseURL: "https://openrouter.ai/api/v1",
	}
	got := cfg.cmdProvider("chat")
	if got.APIKey != "sk-global" {
		t.Errorf("expected APIKey=sk-global, got %q", got.APIKey)
	}
	if got.ProviderType != provider.OpenRouter {
		t.Errorf("expected ProviderType=OpenRouter, got %v", got.ProviderType)
	}
}

// ----- isToolAllowed -----

func Test_isToolAllowed_emptyLists(t *testing.T) {
	if !isToolAllowed("generate_image", nil, nil) {
		t.Error("expected all tools allowed when both lists empty")
	}
	if !isToolAllowed("generate_video", []string{}, []string{}) {
		t.Error("expected all tools allowed when both lists are empty slices")
	}
}

func Test_isToolAllowed_enableMatch(t *testing.T) {
	if !isToolAllowed("generate_image", []string{"generate_image"}, nil) {
		t.Error("expected tool allowed when in enable list")
	}
	if !isToolAllowed("generate_image", []string{"*"}, nil) {
		t.Error("expected tool allowed when enable=*")
	}
}

func Test_isToolAllowed_enableNoMatch(t *testing.T) {
	if isToolAllowed("generate_video", []string{"generate_image"}, nil) {
		t.Error("expected tool denied when not in enable list")
	}
}

func Test_isToolAllowed_disableMatch(t *testing.T) {
	if isToolAllowed("generate_image", nil, []string{"generate_image"}) {
		t.Error("expected tool denied when in disable list")
	}
}

func Test_isToolAllowed_disablePriority(t *testing.T) {
	// disable takes priority over enable
	if isToolAllowed("generate_image", []string{"generate_image"}, []string{"generate_image"}) {
		t.Error("expected disable to take priority over enable")
	}
}

// ----- matchAny -----

func Test_matchAny_exact(t *testing.T) {
	if !matchAny("generate_image", []string{"generate_image"}) {
		t.Error("expected exact match")
	}
}

func Test_matchAny_glob(t *testing.T) {
	if !matchAny("generate_image", []string{"generate_*"}) {
		t.Error("expected glob match for generate_*")
	}
	if !matchAny("generate_video", []string{"generate_*"}) {
		t.Error("expected glob match for generate_*")
	}
}

func Test_matchAny_star(t *testing.T) {
	if !matchAny("anything", []string{"*"}) {
		t.Error("expected * to match everything")
	}
}

func Test_matchAny_noPatterns(t *testing.T) {
	if matchAny("generate_image", nil) {
		t.Error("expected no match when patterns empty")
	}
	if matchAny("generate_image", []string{}) {
		t.Error("expected no match when patterns empty slice")
	}
}

func Test_matchAny_noMatch(t *testing.T) {
	if matchAny("generate_image", []string{"generate_video"}) {
		t.Error("expected no match for different name")
	}
}

// ----- newGenerateImageTool -----

func Test_newGenerateImageTool(t *testing.T) {
	desc := "Custom image description"
	tool := newGenerateImageTool(desc)

	if tool.Name != "generate_image" {
		t.Errorf("expected tool name generate_image, got %q", tool.Name)
	}
	if tool.Description != desc {
		t.Errorf("expected description %q, got %q", desc, tool.Description)
	}
	if len(tool.InputSchema.Properties) == 0 {
		t.Error("expected tool to have input schema properties")
	}
}

// ----- newGenerateVideoTool -----

func Test_newGenerateVideoTool(t *testing.T) {
	desc := "Custom video description"
	tool := newGenerateVideoTool(desc)

	if tool.Name != "generate_video" {
		t.Errorf("expected tool name generate_video, got %q", tool.Name)
	}
	if tool.Description != desc {
		t.Errorf("expected description %q, got %q", desc, tool.Description)
	}
	if len(tool.InputSchema.Properties) == 0 {
		t.Error("expected tool to have input schema properties")
	}
}

// ----- newRemoveWatermarkTool -----

func Test_newRemoveWatermarkTool(t *testing.T) {
	tool := newRemoveWatermarkTool()

	if tool.Name != "remove_watermark" {
		t.Errorf("expected tool name remove_watermark, got %q", tool.Name)
	}
	if tool.Description == "" {
		t.Error("expected non-empty description")
	}
	if len(tool.InputSchema.Properties) == 0 {
		t.Error("expected tool to have input schema properties")
	}
}

// ----- newSearchIdeasTool -----

func Test_newSearchIdeasTool(t *testing.T) {
	tool := newSearchIdeasTool()

	if tool.Name != "search_ideas" {
		t.Errorf("expected tool name search_ideas, got %q", tool.Name)
	}
	if tool.Description == "" {
		t.Error("expected non-empty description")
	}
	if len(tool.InputSchema.Properties) == 0 {
		t.Error("expected tool to have input schema properties")
	}
}

// ----- newDetectTool -----

func Test_newDetectTool(t *testing.T) {
	tool := newDetectTool()

	if tool.Name != "detect_image" {
		t.Errorf("expected tool name detect_image, got %q", tool.Name)
	}
	if tool.Description == "" {
		t.Error("expected non-empty description")
	}
	if len(tool.InputSchema.Properties) == 0 {
		t.Error("expected tool to have input schema properties")
	}
}

// ----- NewServer -----

func TestNewServer_returnsNonNil(t *testing.T) {
	cfg := &Config{
		APIKey:  "sk-test",
		BaseURL: "https://api.openai.com/v1",
		Defaults: &types.ConfigDefaults{
			Image: &types.ImageDefaults{Model: "dall-e-3"},
			Video: &types.VideoDefaults{Model: "veo-2"},
			Audio: &types.AudioDefaults{SpeakModel: "tts-1"},
		},
	}
	s := NewServer(cfg)
	if s == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestNewServer_toolsRegistered(t *testing.T) {
	cfg := &Config{
		APIKey:  "sk-test",
		BaseURL: "https://api.openai.com/v1",
		Defaults: &types.ConfigDefaults{
			Image: &types.ImageDefaults{Model: "dall-e-3"},
			Video: &types.VideoDefaults{Model: "veo-2"},
			Audio: &types.AudioDefaults{SpeakModel: "tts-1"},
		},
	}
	s := NewServer(cfg)

	// We can't directly inspect registered tools on server.MCPServer,
	// but we can verify the server was created without panic.
	// The tool registration happens inside NewServer; if it panics
	// or errors we would have caught it above.
	_ = s
}

func TestNewServer_withEnableList(t *testing.T) {
	cfg := &Config{
		APIKey:       "sk-test",
		BaseURL:      "https://api.openai.com/v1",
		ToolsEnable:  []string{"generate_image"},
		ToolsDisable: nil,
		Defaults: &types.ConfigDefaults{
			Image: &types.ImageDefaults{Model: "dall-e-3"},
			Video: &types.VideoDefaults{Model: "veo-2"},
			Audio: &types.AudioDefaults{SpeakModel: "tts-1"},
		},
	}
	s := NewServer(cfg)
	if s == nil {
		t.Fatal("expected non-nil server with enable list")
	}
}

func TestNewServer_withDisableList(t *testing.T) {
	cfg := &Config{
		APIKey:       "sk-test",
		BaseURL:      "https://api.openai.com/v1",
		ToolsEnable:  nil,
		ToolsDisable: []string{"generate_video"},
		Defaults: &types.ConfigDefaults{
			Image: &types.ImageDefaults{Model: "dall-e-3"},
			Video: &types.VideoDefaults{Model: "veo-2"},
			Audio: &types.AudioDefaults{SpeakModel: "tts-1"},
		},
	}
	s := NewServer(cfg)
	if s == nil {
		t.Fatal("expected non-nil server with disable list")
	}
}

// ----- buildImageDesc / buildVideoDesc (bonus coverage for helpers used by NewServer) -----

func Test_buildImageDesc_withDefaults(t *testing.T) {
	n := 2
	d := &types.ImageDefaults{
		Model:        "dall-e-3",
		Size:         "1024x1024",
		Resolution:   "2k",
		Quality:      "high",
		OutputFormat: "png",
		N:            &n,
	}
	got := buildImageDesc(d, "https://api.openai.com/v1")
	if !strings.Contains(got, "OpenAI") {
		t.Errorf("expected OpenAI mention, got: %s", got)
	}
	if !strings.Contains(got, "dall-e-3") {
		t.Errorf("expected model mention, got: %s", got)
	}
	if !strings.Contains(got, "n = 2") {
		t.Errorf("expected n mention, got: %s", got)
	}
}

func Test_buildVideoDesc_withDefaults(t *testing.T) {
	dur := 10
	d := &types.VideoDefaults{
		Model:      "veo-2",
		Size:       "16:9",
		Resolution: "1080p",
		Duration:   &dur,
	}
	got := buildVideoDesc(d, "https://api.openai.com/v1")
	if !strings.Contains(got, "OpenAI") {
		t.Errorf("expected OpenAI mention, got: %s", got)
	}
	if !strings.Contains(got, "veo-2") {
		t.Errorf("expected model mention, got: %s", got)
	}
	if !strings.Contains(got, "duration = 10s") {
		t.Errorf("expected duration mention, got: %s", got)
	}
}

func Test_buildVideoDesc_openRouterNote(t *testing.T) {
	got := buildVideoDesc(nil, "https://openrouter.ai/api/v1")
	if !strings.Contains(got, "OpenRouter") {
		t.Errorf("expected OpenRouter mention, got: %s", got)
	}
	if !strings.Contains(got, "async") {
		t.Errorf("expected async note for OpenRouter, got: %s", got)
	}
}
