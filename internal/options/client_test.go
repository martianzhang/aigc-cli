package options

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// useConfig points Shared at a throwaway config file for the duration of a test.
func useConfig(t *testing.T, yaml string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	prevFile, prevFlag := Shared.CfgFile, Shared.TimeoutFlag
	Shared.CfgFile = path
	Shared.TimeoutFlag = 0
	t.Cleanup(func() {
		Shared.CfgFile, Shared.TimeoutFlag = prevFile, prevFlag
	})
}

func TestTaskWait(t *testing.T) {
	tests := []struct {
		name   string
		yaml   string
		flag   int
		media  string
		want   time.Duration
		reason string
	}{
		{
			name:  "image uses built-in default",
			media: types.MediaImage,
			want:  types.DefaultImageTaskWait,
		},
		{
			name:  "video uses built-in default",
			media: types.MediaVideo,
			want:  types.DefaultVideoTaskWait,
		},
		{
			name:  "music uses built-in default",
			media: types.MediaMusic,
			want:  types.DefaultMusicTaskWait,
		},
		{
			name:  "midjourney uses defaults.midjourney.timeout",
			yaml:  "defaults:\n  midjourney:\n    timeout: 1800\n",
			media: types.MediaMidjourney,
			want:  30 * time.Minute,
		},
		{
			name:  "unknown media falls back to generic default",
			media: "unknown-media",
			want:  types.FallbackTaskWait,
		},
		{
			name:  "defaults.image.timeout overrides image",
			yaml:  "defaults:\n  image:\n    timeout: 2400\n",
			media: types.MediaImage,
			want:  40 * time.Minute,
		},
		{
			name:  "defaults.video.timeout overrides video only",
			yaml:  "defaults:\n  video:\n    timeout: 5400\n",
			media: types.MediaImage,
			want:  types.DefaultImageTaskWait,
		},
		{
			name:  "defaults.video.timeout applies to video",
			yaml:  "defaults:\n  video:\n    timeout: 5400\n",
			media: types.MediaVideo,
			want:  90 * time.Minute,
		},
		{
			name:  "defaults.music.timeout applies to music",
			yaml:  "defaults:\n  music:\n    timeout: 3600\n",
			media: types.MediaMusic,
			want:  60 * time.Minute,
		},
		{
			name:  "--timeout flag wins over config",
			yaml:  "defaults:\n  image:\n    timeout: 60\n",
			flag:  1800,
			media: types.MediaImage,
			want:  30 * time.Minute,
		},
		{
			name:  "global timeout does not bound task polling",
			yaml:  "timeout: 300\n",
			media: types.MediaImage,
			want:  types.DefaultImageTaskWait,
		},
		{
			name:  "non-positive override is ignored",
			yaml:  "defaults:\n  image:\n    timeout: 0\n",
			media: types.MediaImage,
			want:  types.DefaultImageTaskWait,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			useConfig(t, tc.yaml)
			Shared.TimeoutFlag = tc.flag
			if got := TaskWait(tc.media); got != tc.want {
				t.Errorf("TaskWait(%q) = %v, want %v", tc.media, got, tc.want)
			}
		})
	}
}

func TestResolveTimeoutPriority(t *testing.T) {
	useConfig(t, "timeout: 300\ndefaults:\n  image:\n    timeout: 120\n")

	if got := resolveTimeout("image", 180*time.Second); got != 120*time.Second {
		t.Errorf("image timeout = %v, want 2m", got)
	}
	// No defaults.video.timeout: the global timeout applies to the HTTP client.
	if got := resolveTimeout("video", 180*time.Second); got != 300*time.Second {
		t.Errorf("video timeout = %v, want 5m", got)
	}
	Shared.TimeoutFlag = 45
	if got := resolveTimeout("image", 180*time.Second); got != 45*time.Second {
		t.Errorf("flag timeout = %v, want 45s", got)
	}
}

func TestResolveTimeoutFallback(t *testing.T) {
	useConfig(t, "")
	if got := resolveTimeout("image", 180*time.Second); got != 180*time.Second {
		t.Errorf("fallback timeout = %v, want 3m", got)
	}
}

func TestExplicitTimeout(t *testing.T) {
	useConfig(t, "timeout: 300\ndefaults:\n  image:\n    timeout: 120\n")

	if got, ok := ExplicitTimeout("image"); !ok || got != 120*time.Second {
		t.Errorf("ExplicitTimeout(image) = %v, %v; want 2m, true", got, ok)
	}
	if got, ok := ExplicitTimeout("video"); !ok || got != 300*time.Second {
		t.Errorf("ExplicitTimeout(video) = %v, %v; want 5m, true", got, ok)
	}

	Shared.TimeoutFlag = 45
	if got, ok := ExplicitTimeout("image"); !ok || got != 45*time.Second {
		t.Errorf("ExplicitTimeout(image) with flag = %v, %v; want 45s, true", got, ok)
	}
}

func TestExplicitTimeoutUnset(t *testing.T) {
	useConfig(t, "")
	if got, ok := ExplicitTimeout("image"); ok || got != 0 {
		t.Errorf("ExplicitTimeout(image) = %v, %v; want 0, false", got, ok)
	}
}

func TestHTTPTimeoutFallback(t *testing.T) {
	useConfig(t, "")
	if got := HTTPTimeout("image", 180*time.Second); got != 180*time.Second {
		t.Errorf("HTTPTimeout(image) = %v, want 3m", got)
	}
}

func TestExplicitProviderTimeoutMapsModalities(t *testing.T) {
	useConfig(t, "defaults:\n  image:\n    timeout: 120\n  audio:\n    timeout: 60\n")

	cases := []struct {
		cmd  string
		want time.Duration
	}{
		{ProviderNameImage, 120 * time.Second},
		{ProviderNameUpscale, 120 * time.Second},
		{ProviderNameBackground, 120 * time.Second},
		{ProviderNameDetect, 120 * time.Second},
		{ProviderNameVision, 120 * time.Second},
		{ProviderNameAudioSpeak, 60 * time.Second},
		{ProviderNameAudioSFX, 60 * time.Second},
		{ProviderNameMusic, 0},
	}
	for _, tc := range cases {
		if got := explicitProviderTimeout(tc.cmd); got != tc.want {
			t.Errorf("explicitProviderTimeout(%q) = %v, want %v", tc.cmd, got, tc.want)
		}
	}
}

func TestResolveProviderCarriesExplicitTimeout(t *testing.T) {
	useConfig(t, "timeout: 240\n")
	if p := Shared.ResolveProvider(ProviderNameImage); p == nil || p.Timeout != 240*time.Second {
		t.Errorf("image provider timeout = %v, want 4m", p)
	}

	useConfig(t, "")
	if p := Shared.ResolveProvider(ProviderNameImage); p == nil || p.Timeout != 0 {
		t.Errorf("image provider timeout = %v, want 0", p)
	}
}

func TestTimeoutModality(t *testing.T) {
	cases := map[string]string{
		ProviderNameImage:           "image",
		ProviderNameUpscale:         "image",
		ProviderNameBackground:      "image",
		ProviderNameDetect:          "image",
		ProviderNameVision:          "image",
		ProviderNameAudioSpeak:      "audio",
		ProviderNameAudioTranscribe: "audio",
		ProviderNameAudioSFX:        "audio",
		ProviderNameVideo:           "video",
		ProviderNameMusic:           "music",
	}
	for cmd, want := range cases {
		if got := timeoutModality(cmd); got != want {
			t.Errorf("timeoutModality(%q) = %q, want %q", cmd, got, want)
		}
	}
}

func TestModalityTimeoutAllKeys(t *testing.T) {
	useConfig(t, `timeout: 900
defaults:
  image:
    timeout: 11
  video:
    timeout: 22
  midjourney:
    timeout: 33
  music:
    timeout: 44
  audio:
    timeout: 55
`)
	want := map[string]int{
		"image":      11,
		"video":      22,
		"midjourney": 33,
		"music":      44,
		"audio":      55,
	}
	for key, seconds := range want {
		got := modalityTimeout(key)
		if got == nil || *got != seconds {
			t.Errorf("modalityTimeout(%q) = %v, want %d", key, got, seconds)
		}
	}
	if got := modalityTimeout("chat"); got != nil {
		t.Errorf("modalityTimeout(chat) = %d, want nil", *got)
	}
}

func TestModalityTimeoutNilDefaults(t *testing.T) {
	useConfig(t, "timeout: 900\n")
	if got := modalityTimeout("image"); got != nil {
		t.Errorf("modalityTimeout(image) = %d, want nil", *got)
	}
}
