package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func sfxRequest(args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Name = "generate_sfx"
	req.Params.Arguments = args
	return req
}

func TestGenerateSFXTool_Definition(t *testing.T) {
	tool := newGenerateSFXTool("desc")
	if tool.Name != "generate_sfx" {
		t.Errorf("name = %q, want generate_sfx", tool.Name)
	}
	if tool.Description == "" {
		t.Error("description is empty")
	}
	for _, key := range []string{"input", "duration", "loop", "format", "seed"} {
		if _, ok := tool.InputSchema.Properties[key]; !ok {
			t.Errorf("missing property %q", key)
		}
	}
}

func TestGenerateSFXHandler_MissingInput(t *testing.T) {
	cfg := &Config{Defaults: &types.ConfigDefaults{}}
	res, err := generateSFXHandler(cfg)(context.Background(), sfxRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if !res.IsError || !strings.Contains(toolResultText(t, res), "input is required") {
		t.Fatalf("result = %+v", res)
	}
}

func TestGenerateSFXHandler_LocalWithoutModelsDir(t *testing.T) {
	cfg := &Config{
		Defaults: &types.ConfigDefaults{},
		CmdProviders: map[string]*provider.EffectiveProvider{
			"audio.sfx": {Name: "local", Type: types.ProviderLocal},
		},
	}
	res, err := generateSFXHandler(cfg)(context.Background(), sfxRequest(map[string]any{
		"input": "door slam",
	}))
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if !res.IsError || !strings.Contains(toolResultText(t, res), "models directory") {
		t.Fatalf("result = %+v", res)
	}
}

func TestGenerateSFXHandler_OnlineElevenLabs(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Header.Get("xi-api-key") != "xi-test" {
			t.Errorf("xi-api-key = %q", r.Header.Get("xi-api-key"))
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("fake-mp3"))
	}))
	defer srv.Close()

	out := t.TempDir()
	cfg := &Config{
		Output:   out,
		Defaults: &types.ConfigDefaults{},
		CmdProviders: map[string]*provider.EffectiveProvider{
			"audio.sfx": {
				Name:         "elevenlabs",
				BaseURL:      srv.URL,
				APIKey:       "xi-test",
				ProviderType: provider.ElevenLabs,
			},
		},
	}

	res, err := generateSFXHandler(cfg)(context.Background(), sfxRequest(map[string]any{
		"input":    "door slam",
		"duration": 1.5,
		"format":   "mp3",
	}))
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res.IsError {
		t.Fatalf("handler error: %s", toolResultText(t, res))
	}
	if gotPath != "/v1/sound-generation" {
		t.Errorf("path = %q", gotPath)
	}
	entries, err := os.ReadDir(out)
	if err != nil || len(entries) != 1 {
		t.Fatalf("output files = %v, %v", entries, err)
	}
	if !strings.HasSuffix(entries[0].Name(), ".mp3") {
		t.Errorf("saved file = %q, want .mp3", entries[0].Name())
	}
	if _, err := os.Stat(filepath.Join(out, entries[0].Name())); err != nil {
		t.Errorf("saved file missing: %v", err)
	}
}

func TestBuildSFXDesc(t *testing.T) {
	d := &types.AudioDefaults{SFX: &types.AudioSFXDefaults{Model: "eleven_text_to_sound_v2", Format: "mp3", Provider: "elevenlabs"}}
	desc := buildSFXDesc(d, "https://api.elevenlabs.io")
	for _, want := range []string{"sound effect", "eleven_text_to_sound_v2", "provider=local"} {
		if !strings.Contains(desc, want) {
			t.Errorf("description missing %q: %s", want, desc)
		}
	}
}
