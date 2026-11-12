package options

import (
	"testing"
	"time"
)

func TestExplicitProviderTimeoutMapsModalities(t *testing.T) {
	useConfig(t, `defaults:
  image: {timeout: 240}
  video: {timeout: 300}
  audio: {timeout: 60}
`)
	cases := []struct {
		cmd  string
		want time.Duration
	}{
		{ProviderNameImage, 240 * time.Second},
		{ProviderNameUpscale, 240 * time.Second},
		{ProviderNameBackground, 240 * time.Second},
		{ProviderNameDetect, 240 * time.Second},
		{ProviderNameVision, 240 * time.Second},
		{ProviderNameVideo, 300 * time.Second},
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
	want := map[string]int{"image": 11, "video": 22, "midjourney": 33, "music": 44, "audio": 55}
	for key, seconds := range want {
		got := ModalityTimeout(key)
		if got == nil || *got != seconds {
			t.Errorf("ModalityTimeout(%q) = %v, want %d", key, got, seconds)
		}
	}
	if got := ModalityTimeout("chat"); got != nil {
		t.Errorf("ModalityTimeout(chat) = %d, want nil", *got)
	}
}

func TestModalityTimeoutNilDefaults(t *testing.T) {
	useConfig(t, "timeout: 900\n")
	if got := ModalityTimeout("image"); got != nil {
		t.Errorf("ModalityTimeout(image) = %d, want nil", *got)
	}
}
