package music

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestBuildFunMusicBody(t *testing.T) {
	instrumental := true
	duration := 120

	cases := []struct {
		name    string
		req     *types.MusicGenerateRequest
		want    map[string]any
		wantErr bool
	}{
		{
			name: "prompt only uses default model",
			req:  &types.MusicGenerateRequest{Prompt: "夏日清新民谣"},
			want: map[string]any{
				"model": "fun-music-v1",
				"input": map[string]any{"prompt": "夏日清新民谣"},
			},
		},
		{
			name: "style folds into prompt",
			req:  &types.MusicGenerateRequest{Prompt: "城市", Style: "lofi"},
			want: map[string]any{
				"model": "fun-music-v1",
				"input": map[string]any{"prompt": "城市，lofi"},
			},
		},
		{
			name: "style only becomes prompt",
			req:  &types.MusicGenerateRequest{Style: "古风"},
			want: map[string]any{
				"model": "fun-music-v1",
				"input": map[string]any{"prompt": "古风"},
			},
		},
		{
			name: "lyrics passed through",
			req:  &types.MusicGenerateRequest{Lyrics: "[verse]hi"},
			want: map[string]any{
				"model": "fun-music-v1",
				"input": map[string]any{"lyrics": "[verse]hi"},
			},
		},
		{
			name: "instrumental drops lyrics",
			req:  &types.MusicGenerateRequest{Prompt: "钢琴", Lyrics: "[verse]x", Instrumental: &instrumental},
			want: map[string]any{
				"model": "fun-music-v1",
				"input": map[string]any{"prompt": "钢琴", "is_instrumental": true},
			},
		},
		{
			name: "fun-music model is preserved",
			req:  &types.MusicGenerateRequest{Model: "fun-music-preview", Prompt: "x"},
			want: map[string]any{
				"model": "fun-music-preview",
				"input": map[string]any{"prompt": "x"},
			},
		},
		{
			name: "foreign model falls back",
			req:  &types.MusicGenerateRequest{Model: "suno", Prompt: "x"},
			want: map[string]any{
				"model": "fun-music-v1",
				"input": map[string]any{"prompt": "x"},
			},
		},
		{
			name: "duration and title are dropped",
			req:  &types.MusicGenerateRequest{Prompt: "x", Duration: &duration, Title: "t"},
			want: map[string]any{
				"model": "fun-music-v1",
				"input": map[string]any{"prompt": "x"},
			},
		},
		{
			name:    "no prompt and no lyrics is an error",
			req:     &types.MusicGenerateRequest{},
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := BuildFunMusicBody(c.req)
			if c.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("BuildFunMusicBody: %v", err)
			}
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(c.want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("body mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestBuildFunMusicBodyForwardsRawJSON(t *testing.T) {
	const raw = `{"model":"fun-music-v1","input":{"prompt":"x","gender":"male"},"vendor_future_key":9}`
	got, err := BuildFunMusicBody(&types.MusicGenerateRequest{RawJSON: []byte(raw)})
	if err != nil {
		t.Fatalf("BuildFunMusicBody: %v", err)
	}
	gotJSON, _ := json.Marshal(got)
	if string(gotJSON) != raw {
		t.Errorf("raw --json body = %s, want %s", gotJSON, raw)
	}
}

func TestBuildFunMusicCurl(t *testing.T) {
	curl := buildFunMusicCurl(
		"https://abc.cn-beijing.maas.aliyuncs.com/compatible-mode/v1",
		"sk-secret-key",
		map[string]any{"model": "fun-music-v1", "input": map[string]any{"prompt": "x"}},
	)

	if !strings.Contains(curl, "https://abc.cn-beijing.maas.aliyuncs.com/api/v1/services/audio/music/generation") {
		t.Errorf("curl missing native endpoint:\n%s", curl)
	}
	if !strings.Contains(curl, `-H "Authorization: Bearer`) {
		t.Errorf("curl missing auth header:\n%s", curl)
	}
	if strings.Contains(curl, "sk-secret-key") {
		t.Errorf("curl leaked the raw api key:\n%s", curl)
	}
}
