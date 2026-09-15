package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestFunMusicEndpoint(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "compatible-mode base url keeps host only",
			in:   "https://llm-abc.cn-beijing.maas.aliyuncs.com/compatible-mode/v1",
			want: "https://llm-abc.cn-beijing.maas.aliyuncs.com/api/v1/services/audio/music/generation",
		},
		{
			name: "workspace host without version",
			in:   "https://abc.cn-beijing.maas.aliyuncs.com",
			want: "https://abc.cn-beijing.maas.aliyuncs.com/api/v1/services/audio/music/generation",
		},
		{
			name: "legacy dashscope host",
			in:   "https://dashscope.aliyuncs.com",
			want: "https://dashscope.aliyuncs.com/api/v1/services/audio/music/generation",
		},
		{
			name: "empty falls back to dashscope",
			in:   "",
			want: "https://dashscope.aliyuncs.com/api/v1/services/audio/music/generation",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FunMusicEndpoint(c.in); got != c.want {
				t.Errorf("FunMusicEndpoint(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestFunMusicGenerate(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{
			"request_id": "req_1",
			"output": {
				"audio": {"url": "https://oss.example.com/a.mp3", "id": "audio_1", "expires_at": 1774936147},
				"extra_info": {"channels": 2, "sample_rate": "48000", "lyrics": "[verse]hi"},
				"finish_reason": "stop"
			},
			"usage": {"duration": 200}
		}`))
	}))
	defer srv.Close()

	c := NewWithProvider("sk-test", srv.URL, "", types.ProviderOpenAI)
	resp, err := c.FunMusicGenerate(map[string]any{
		"model": "fun-music-v1",
		"input": map[string]any{"prompt": "folk"},
	})
	if err != nil {
		t.Fatalf("FunMusicGenerate: %v", err)
	}

	if gotPath != funMusicPath {
		t.Errorf("path = %q, want %q", gotPath, funMusicPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("Authorization = %q, want bearer", gotAuth)
	}
	if gotBody["model"] != "fun-music-v1" {
		t.Errorf("body model = %v, want fun-music-v1", gotBody["model"])
	}
	if resp.Output.Audio.URL != "https://oss.example.com/a.mp3" {
		t.Errorf("audio url = %q", resp.Output.Audio.URL)
	}
	if resp.Usage.Duration != 200 {
		t.Errorf("duration = %d, want 200", resp.Usage.Duration)
	}

	track := resp.Track()
	if track.AudioURL != "https://oss.example.com/a.mp3" {
		t.Errorf("track url = %q", track.AudioURL)
	}
	if track.DurationSeconds != "200" {
		t.Errorf("track duration = %q, want \"200\"", track.DurationSeconds)
	}
}

func TestFunMusicGenerate_errorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":"InvalidParameter","message":"bad param","request_id":"r"}`))
	}))
	defer srv.Close()

	c := NewWithProvider("sk-test", srv.URL, "", types.ProviderOpenAI)
	if _, err := c.FunMusicGenerate(map[string]any{}); err == nil {
		t.Fatal("expected error for error envelope, got nil")
	}
}

func TestFunMusicGenerate_missingAudioURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"request_id":"r","output":{"audio":{}}}`))
	}))
	defer srv.Close()

	c := NewWithProvider("sk-test", srv.URL, "", types.ProviderOpenAI)
	if _, err := c.FunMusicGenerate(map[string]any{}); err == nil {
		t.Fatal("expected error when audio url is missing, got nil")
	}
}
