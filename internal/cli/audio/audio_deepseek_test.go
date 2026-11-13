package audio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeDeepSeekVoice(t *testing.T) {
	for _, voice := range []string{"", "stella", "TIDE"} {
		if _, err := normalizeDeepSeekVoice(voice); err != nil {
			t.Errorf("normalizeDeepSeekVoice(%q) error = %v", voice, err)
		}
	}
	if got, _ := normalizeDeepSeekVoice(""); got != "stella" {
		t.Errorf("default voice = %q, want stella", got)
	}
	if _, err := normalizeDeepSeekVoice("alloy"); err == nil || !strings.Contains(err.Error(), "stella") {
		t.Errorf("alloy should fail with a hint, got %v", err)
	}
}

func TestNormalizeDeepSeekTTSFormat(t *testing.T) {
	if got, _ := normalizeDeepSeekTTSFormat(""); got != "wav" {
		t.Errorf("default format = %q, want wav", got)
	}
	if got, _ := normalizeDeepSeekTTSFormat("PCM"); got != "pcm" {
		t.Errorf("format = %q, want pcm", got)
	}
	if _, err := normalizeDeepSeekTTSFormat("mp3"); err == nil {
		t.Error("mp3 should be rejected")
	}
}

func TestValidateDeepSeekAudioInput(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "speech.wav")
	if err := os.WriteFile(wav, []byte("RIFF"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateDeepSeekAudioInput(nil, wav); err != nil {
		t.Errorf("wav should be accepted: %v", err)
	}

	mp3 := filepath.Join(dir, "speech.mp3")
	if err := os.WriteFile(mp3, []byte("ID3"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateDeepSeekAudioInput(nil, mp3); err == nil {
		t.Error("mp3 should be rejected")
	}

	large := filepath.Join(dir, "large.wav")
	if err := os.WriteFile(large, make([]byte, deepSeekASRMaxBytes+1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateDeepSeekAudioInput(nil, large); err == nil || !strings.Contains(err.Error(), "8 MiB") {
		t.Errorf("oversized file should fail, got %v", err)
	}

	if err := validateDeepSeekAudioInput(nil, "not-a-file"); err == nil {
		t.Error("missing file should fail")
	}
}
