package music

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func overlayBailianProvider() *provider.EffectiveProvider {
	return &provider.EffectiveProvider{BaseURL: "https://ws.cn-beijing.maas.aliyuncs.com"}
}

func overlayOpenRouterProvider() *provider.EffectiveProvider {
	return &provider.EffectiveProvider{BaseURL: "https://openrouter.ai/api/v1"}
}

func unmarshalObject(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("unmarshal %q: %v", s, err)
	}
	return m
}

func TestApplyMusicJSONOverlay(t *testing.T) {
	cases := []struct {
		name    string
		req     *types.MusicGenerateRequest
		changed map[string]bool
		p       *provider.EffectiveProvider
		raw     string
		want    string
	}{
		{
			name:    "suno prompt only overrides prompt and preserves vendor keys",
			req:     &types.MusicGenerateRequest{Model: "suno", Prompt: "rock"},
			changed: map[string]bool{"prompt": true},
			raw:     `{"model":"suno","prompt":"jazz","vendor_future_key":9}`,
			want:    `{"model":"suno","prompt":"rock","vendor_future_key":9}`,
		},
		{
			name:    "suno lyrics present flips prompt to style",
			req:     &types.MusicGenerateRequest{Model: "suno", Prompt: "rock", Lyrics: "la la"},
			changed: map[string]bool{"prompt": true, "lyrics": true},
			raw:     `{"model":"suno","prompt":"jazz","custom":false}`,
			want:    `{"model":"suno","custom":true,"prompt":"la la","style":"rock"}`,
		},
		{
			name:    "suno style only with no lyrics feeds prompt",
			req:     &types.MusicGenerateRequest{Model: "suno", Style: "jazz"},
			changed: map[string]bool{"style": true},
			raw:     `{"model":"suno","prompt":"old"}`,
			want:    `{"model":"suno","prompt":"jazz"}`,
		},
		{
			name:    "suno title overlay requires custom mode",
			req:     &types.MusicGenerateRequest{Model: "suno", Lyrics: "la", Title: "My Song"},
			changed: map[string]bool{"lyrics": true, "title": true},
			raw:     `{"model":"suno","custom":true,"prompt":"old"}`,
			want:    `{"model":"suno","custom":true,"prompt":"la","title":"My Song"}`,
		},
		{
			name: "flowmusic sound_prompt and length, format dropped",
			req: &types.MusicGenerateRequest{
				Model:    "flowmusic",
				Prompt:   "rock",
				Duration: ptr(180),
				Format:   "wav",
			},
			changed: map[string]bool{"prompt": true, "duration": true, "format": true},
			raw:     `{"model":"flowmusic","sound_prompt":"old","length":120}`,
			want:    `{"model":"flowmusic","sound_prompt":"rock","length":180}`,
		},
		{
			name:    "flowmusic format only is a no-op",
			req:     &types.MusicGenerateRequest{Model: "flowmusic", Format: "wav"},
			changed: map[string]bool{"format": true},
			raw:     `{"model":"flowmusic","sound_prompt":"x"}`,
			want:    `{"model":"flowmusic","sound_prompt":"x"}`,
		},
		{
			name:    "fun-music input prompt and lyrics preserve other input keys",
			req:     &types.MusicGenerateRequest{Model: "fun-music-v1", Prompt: "夏日", Lyrics: "[v]x"},
			changed: map[string]bool{"prompt": true, "lyrics": true},
			p:       overlayBailianProvider(),
			raw:     `{"model":"fun-music-v1","input":{"prompt":"old","gender":"male"}}`,
			want:    `{"model":"fun-music-v1","input":{"prompt":"夏日","lyrics":"[v]x","gender":"male"}}`,
		},
		{
			name:    "fun-music duration and title are dropped",
			req:     &types.MusicGenerateRequest{Model: "fun-music-v1", Duration: ptr(60), Title: "t"},
			changed: map[string]bool{"duration": true, "title": true},
			p:       overlayBailianProvider(),
			raw:     `{"model":"fun-music-v1","input":{"prompt":"x"}}`,
			want:    `{"model":"fun-music-v1","input":{"prompt":"x"}}`,
		},
		{
			name:    "openrouter model and audio format only; messages untouched",
			req:     &types.MusicGenerateRequest{Model: "google/lyria-3-pro-preview", Prompt: "rock", Format: "wav"},
			changed: map[string]bool{"model": true, "prompt": true, "format": true},
			p:       overlayOpenRouterProvider(),
			raw:     `{"model":"google/lyria-3-clip-preview","audio":{"format":"mp3"},"messages":[{"role":"user","content":"raw"}],"vendor":1}`,
			want:    `{"model":"google/lyria-3-pro-preview","audio":{"format":"wav"},"messages":[{"role":"user","content":"raw"}],"vendor":1}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.req.RawJSON = json.RawMessage(c.raw)
			if err := applyMusicJSONOverlay(c.changed, c.req, c.p); err != nil {
				t.Fatalf("applyMusicJSONOverlay() error = %v", err)
			}
			got := unmarshalObject(t, string(c.req.RawJSON))
			want := unmarshalObject(t, c.want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("overlay body\n got: %s\nwant: %s", c.req.RawJSON, c.want)
			}
		})
	}
}

func TestApplyMusicJSONOverlayNoFlagsIsByteIdentical(t *testing.T) {
	const raw = `{"model":"suno","prompt":"jazz","vendor":{"a":[1,2,3]}}`
	req := &types.MusicGenerateRequest{RawJSON: json.RawMessage(raw)}
	if err := applyMusicJSONOverlay(map[string]bool{}, req, nil); err != nil {
		t.Fatalf("applyMusicJSONOverlay() error = %v", err)
	}
	if string(req.RawJSON) != raw {
		t.Errorf("no-flag overlay changed the body:\n got: %s\nwant: %s", req.RawJSON, raw)
	}
}

func TestChangedMusicFlagsFiltersBehavioralFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "generate"}
	f := cmd.Flags()
	f.String("model", "", "")
	f.String("prompt", "", "")
	f.String("style", "", "")
	f.String("title", "", "")
	f.String("lyrics", "", "")
	f.Bool("instrumental", false, "")
	f.Int("duration", 0, "")
	f.String("format", "", "")
	f.String("json", "", "")
	f.Bool("dry-run", false, "")
	f.String("provider", "", "")
	f.String("api-key", "", "")
	f.String("api-base", "", "")
	f.String("http-proxy", "", "")
	f.String("config", "", "")
	f.String("output", "", "")
	f.Bool("verbose", false, "")
	f.Int("timeout", 0, "")
	f.Bool("print-config", false, "")

	for _, name := range []string{"prompt", "duration", "dry-run", "provider", "verbose", "print-config"} {
		if err := f.Set(name, "1"); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
	}

	want := map[string]bool{"prompt": true, "duration": true}
	if got := changedMusicFlags(cmd); !reflect.DeepEqual(got, want) {
		t.Errorf("changedMusicFlags() = %v, want %v", got, want)
	}
}
