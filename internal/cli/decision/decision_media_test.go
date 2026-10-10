package decision

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
	return path
}

func wantDataURL(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func TestResolveAudioVideo(t *testing.T) {
	dir := t.TempDir()
	wav := []byte("RIFF\x24\x00\x00\x00WAVEfmt ")
	mp4 := []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom")
	wavPath := writeFixture(t, dir, "call.wav", wav)
	mp4Path := writeFixture(t, dir, "dashcam.mp4", mp4)
	wavURI := "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(wav)
	mp4URI := "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(mp4)

	t.Run("nil and empty inputs", func(t *testing.T) {
		if got, err := resolveAudio(nil, nil); err != nil || got != nil {
			t.Errorf("resolveAudio(nil,nil) = (%v,%v), want (nil,nil)", got, err)
		}
		if got, err := resolveVideos([]string{}, []string{}); err != nil || got != nil {
			t.Errorf("resolveVideos([],[]) = (%v,%v), want (nil,nil)", got, err)
		}
	})

	t.Run("local wav encoded to an audio data URL", func(t *testing.T) {
		got, err := resolveAudio([]string{wavPath}, nil)
		if err != nil {
			t.Fatalf("resolveAudio(file) = %v, want nil", err)
		}
		if len(got) != 1 || got[0] != wantDataURL("audio/wav", wav) {
			t.Errorf("resolveAudio(file) = %v, want [%s]", got, wantDataURL("audio/wav", wav))
		}
	})

	t.Run("local mp4 encoded to a video data URL", func(t *testing.T) {
		got, err := resolveVideos([]string{mp4Path}, nil)
		if err != nil {
			t.Fatalf("resolveVideos(file) = %v, want nil", err)
		}
		if len(got) != 1 || got[0] != wantDataURL("video/mp4", mp4) {
			t.Errorf("resolveVideos(file) = %v, want [%s]", got, wantDataURL("video/mp4", mp4))
		}
	})

	t.Run("data URI passes through unchanged", func(t *testing.T) {
		got, err := resolveAudio([]string{wavURI}, nil)
		if err != nil {
			t.Fatalf("resolveAudio(dataURI) = %v, want nil", err)
		}
		if len(got) != 1 || got[0] != wavURI {
			t.Errorf("resolveAudio(dataURI) = %v, want [%s]", got, wavURI)
		}
		got, err = resolveVideos([]string{mp4URI}, nil)
		if err != nil {
			t.Fatalf("resolveVideos(dataURI) = %v, want nil", err)
		}
		if len(got) != 1 || got[0] != mp4URI {
			t.Errorf("resolveVideos(dataURI) = %v, want [%s]", got, mp4URI)
		}
	})

	t.Run("unknown extension sniffed as audio is accepted", func(t *testing.T) {
		binPath := writeFixture(t, dir, "clip.bin", wav)
		got, err := resolveAudio([]string{binPath}, nil)
		if err != nil {
			t.Fatalf("resolveAudio(sniffed) = %v, want nil", err)
		}
		if len(got) != 1 || !strings.HasPrefix(got[0], "data:audio/") {
			t.Errorf("resolveAudio(sniffed) = %v, want an audio data URL", got)
		}
	})

	t.Run("blank entries skipped", func(t *testing.T) {
		got, err := resolveVideos([]string{"", "   ", mp4Path}, nil)
		if err != nil {
			t.Fatalf("resolveVideos(blank) = %v, want nil", err)
		}
		if len(got) != 1 || got[0] != wantDataURL("video/mp4", mp4) {
			t.Errorf("resolveVideos(blank) = %v, want 1 video", got)
		}
	})

	t.Run("non-media file rejected", func(t *testing.T) {
		txtPath := writeFixture(t, dir, "notes.txt", []byte("just text"))
		if _, err := resolveAudio([]string{txtPath}, nil); err == nil || !strings.Contains(err.Error(), "not a recognized audio file") {
			t.Errorf("resolveAudio(txt) error = %v, want rejection", err)
		}
	})

	t.Run("URL rejected", func(t *testing.T) {
		if _, err := resolveVideos([]string{"https://example.com/clip.mp4"}, nil); err == nil || !strings.Contains(err.Error(), "does not fetch URLs") {
			t.Errorf("resolveVideos(url) error = %v, want URL rejection", err)
		}
	})

	t.Run("CLI overrides bank", func(t *testing.T) {
		got, err := resolveAudio([]string{wavPath}, []string{"bank-audio.wav"})
		if err != nil {
			t.Fatalf("resolveAudio(cli+bank) = %v, want nil", err)
		}
		if len(got) != 1 || got[0] != wantDataURL("audio/wav", wav) {
			t.Errorf("resolveAudio(cli+bank) = %v, want only the CLI clip", got)
		}
	})

	t.Run("bank used when CLI empty", func(t *testing.T) {
		got, err := resolveVideos(nil, []string{mp4Path})
		if err != nil {
			t.Fatalf("resolveVideos(bank) = %v, want nil", err)
		}
		if len(got) != 1 || got[0] != wantDataURL("video/mp4", mp4) {
			t.Errorf("resolveVideos(bank) = %v, want the bank clip", got)
		}
	})

	t.Run("malformed data URI rejected", func(t *testing.T) {
		if _, err := resolveAudio([]string{"data:audio/wav;base64"}, nil); err == nil || !strings.Contains(err.Error(), "missing comma") {
			t.Errorf("resolveAudio(bad data URI) error = %v, want missing-comma error", err)
		}
	})
}

func TestValidateMediaDataURI(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		wantErr string
	}{
		{name: "valid", uri: "data:audio/wav;base64,aGVsbG8="},
		{name: "missing comma", uri: "data:audio/wav;base64", wantErr: "missing comma"},
		{name: "non-base64", uri: "data:audio/wav;hex,0a0b", wantErr: "must be base64-encoded"},
		{name: "empty payload", uri: "data:audio/wav;base64,", wantErr: "contains no data"},
	}
	for _, tc := range tests {
		err := validateMediaDataURI(tc.uri)
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("%s: validateMediaDataURI(%q) = %v, want nil", tc.name, tc.uri, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: validateMediaDataURI(%q) error = %v, want %q", tc.name, tc.uri, err, tc.wantErr)
		}
	}
}
