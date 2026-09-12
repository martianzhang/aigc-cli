package cmd

import (
	"reflect"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func musicBoolPtr(v bool) *bool { return &v }

func TestBuildMusicBody(t *testing.T) {
	cases := []struct {
		name    string
		req     *types.MusicGenerateRequest
		want    map[string]any
		wantErr bool
	}{
		{
			name: "suno prompt only",
			req:  &types.MusicGenerateRequest{Model: "suno", Prompt: "city pop"},
			want: map[string]any{
				"model":        "suno",
				"custom":       false,
				"prompt":       "city pop",
				"instrumental": false,
				"version":      "v6",
			},
		},
		{
			name: "suno with lyrics uses custom mode",
			req: &types.MusicGenerateRequest{
				Model:  "suno",
				Prompt: "pop",
				Lyrics: "la la",
				Title:  "My Song",
			},
			want: map[string]any{
				"model":        "suno",
				"custom":       true,
				"prompt":       "la la",
				"style":        "pop",
				"title":        "My Song",
				"instrumental": false,
				"version":      "v6",
			},
		},
		{
			name: "flowmusic prompt only",
			req:  &types.MusicGenerateRequest{Model: "flowmusic", Prompt: "rock"},
			want: map[string]any{
				"model":        "flowmusic",
				"sound_prompt": "rock",
				"length":       120,
			},
		},
		{
			name: "flowmusic duration and lyrics",
			req: &types.MusicGenerateRequest{
				Model:    "flowmusic",
				Prompt:   "rock",
				Lyrics:   "hey",
				Duration: ptr(180),
			},
			want: map[string]any{
				"model":        "flowmusic",
				"sound_prompt": "rock",
				"lyrics":       "hey",
				"length":       180,
			},
		},
		{
			name: "flowmusic instrumental drops lyrics",
			req: &types.MusicGenerateRequest{
				Model:        "flowmusic",
				Prompt:       "rock",
				Lyrics:       "hey",
				Instrumental: musicBoolPtr(true),
			},
			want: map[string]any{
				"model":        "flowmusic",
				"sound_prompt": "rock",
				"length":       120,
			},
		},
		{
			name: "json overlay wins",
			req: &types.MusicGenerateRequest{
				Model:  "suno",
				Prompt: "x",
				Extras: map[string]any{"length": 200, "custom": true},
			},
			want: map[string]any{
				"model":        "suno",
				"prompt":       "x",
				"custom":       true,
				"instrumental": false,
				"version":      "v6",
				"length":       200,
			},
		},
		{
			name:    "suno empty prompt errors",
			req:     &types.MusicGenerateRequest{Model: "suno"},
			wantErr: true,
		},
		{
			name:    "flowmusic empty sound_prompt and lyrics errors",
			req:     &types.MusicGenerateRequest{Model: "flowmusic"},
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := buildMusicBody(c.req)
			if c.wantErr {
				if err == nil {
					t.Fatalf("buildMusicBody() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildMusicBody() error = %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("buildMusicBody() = %#v, want %#v", got, c.want)
			}
		})
	}
}

func TestBuildMusicCurl(t *testing.T) {
	body := map[string]any{"model": "suno", "prompt": "city pop"}
	curl := buildMusicCurl("https://api.apimart.ai", "test-key-123", body)

	if !strings.Contains(curl, "/v1/music/generations") {
		t.Errorf("curl should contain /v1/music/generations, got:\n%s", curl)
	}
	if !strings.Contains(curl, "test-key-123") {
		t.Error("curl should contain API key")
	}
	if !strings.Contains(curl, "city pop") {
		t.Error("curl should contain prompt")
	}
}

func TestBuildMusicCurl_existingVersionSuffix(t *testing.T) {
	curl := buildMusicCurl("https://api.apimart.ai/v1", "test-key", map[string]any{"model": "suno"})
	if !strings.Contains(curl, "https://api.apimart.ai/v1/music/generations") {
		t.Errorf("curl should not duplicate version, got:\n%s", curl)
	}
	if strings.Contains(curl, "/v1/v1/") {
		t.Errorf("curl duplicated version suffix:\n%s", curl)
	}
}

func TestGenerateMusicAndSave_nilRequest(t *testing.T) {
	if _, err := generateMusicAndSave(nil, nil); err == nil {
		t.Fatal("generateMusicAndSave(nil, nil) expected error, got nil")
	}
}

func TestBuildOpenRouterMusicReq(t *testing.T) {
	cases := []struct {
		name            string
		req             *types.MusicGenerateRequest
		wantModel       string
		wantFormat      string
		contentContains []string
	}{
		{
			name:            "defaults",
			req:             &types.MusicGenerateRequest{Prompt: "lofi"},
			wantModel:       openRouterMusicDefaultModel,
			wantFormat:      "mp3",
			contentContains: []string{"lofi"},
		},
		{
			name: "instrumental lyrics duration folded into text",
			req: &types.MusicGenerateRequest{
				Prompt:       "rock",
				Lyrics:       "la la",
				Instrumental: musicBoolPtr(true),
				Duration:     ptr(30),
			},
			wantModel:       openRouterMusicDefaultModel,
			wantFormat:      "mp3",
			contentContains: []string{"[Instrumental] rock", "Lyrics:\nla la", "Target duration: about 30 seconds."},
		},
		{
			name:            "style fallback and format override",
			req:             &types.MusicGenerateRequest{Style: "jazz", Format: "wav"},
			wantModel:       openRouterMusicDefaultModel,
			wantFormat:      "wav",
			contentContains: []string{"jazz"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildOpenRouterMusicReq(c.req)
			if got.Model != c.wantModel {
				t.Errorf("Model = %q, want %q", got.Model, c.wantModel)
			}
			if !got.Stream {
				t.Error("Stream should be true")
			}
			if len(got.Modalities) != 2 || got.Modalities[0] != "text" || got.Modalities[1] != "audio" {
				t.Errorf("Modalities = %v, want [text audio]", got.Modalities)
			}
			if got.Audio == nil || got.Audio.Format != c.wantFormat {
				t.Errorf("Audio.Format = %v, want %q", got.Audio, c.wantFormat)
			}
			if len(got.Messages) != 1 || got.Messages[0].Role != "user" {
				t.Fatalf("Messages = %v, want one user message", got.Messages)
			}
			for _, sub := range c.contentContains {
				if !strings.Contains(got.Messages[0].Content, sub) {
					t.Errorf("message content %q missing %q", got.Messages[0].Content, sub)
				}
			}
		})
	}
}

func TestBuildOpenRouterMusicCurl(t *testing.T) {
	curl := buildOpenRouterMusicCurl("https://openrouter.ai/api/v1", "sk-x", &types.MusicGenerateRequest{Prompt: "lofi"})
	for _, want := range []string{"/chat/completions", "curl -N -X POST", `"stream":true`, `"audio"`} {
		if !strings.Contains(curl, want) {
			t.Errorf("curl missing %q, got:\n%s", want, curl)
		}
	}
}

func TestBuildOpenRouterMusicCurl_addsVersionSuffix(t *testing.T) {
	curl := buildOpenRouterMusicCurl("https://openrouter.ai", "sk-x", &types.MusicGenerateRequest{Prompt: "lofi"})
	if !strings.Contains(curl, "https://openrouter.ai/v1/chat/completions") {
		t.Errorf("curl should add /v1, got:\n%s", curl)
	}
}
