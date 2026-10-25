package audio

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// withAudioDryRunFlags resets the audio flags around one dry-run test and
// points outputs at a fresh temp directory.
func withAudioDryRunFlags(t *testing.T) string {
	t.Helper()
	oldDry, oldOut := audioDryRun, options.Shared.OutputDir
	oldSpeakInput, oldSpeakModel, oldSpeakVoice := audioSpeechInput, audioSpeechModel, audioSpeechVoice
	oldTranscribeInput, oldTranscribeModel := audioTranscribeInput, audioTranscribeModel
	t.Cleanup(func() {
		audioDryRun, options.Shared.OutputDir = oldDry, oldOut
		audioSpeechInput, audioSpeechModel, audioSpeechVoice = oldSpeakInput, oldSpeakModel, oldSpeakVoice
		audioTranscribeInput, audioTranscribeModel = oldTranscribeInput, oldTranscribeModel
	})

	dir := t.TempDir()
	audioDryRun = true
	options.Shared.OutputDir = dir
	audioSpeechInput, audioSpeechModel, audioSpeechVoice = "", "", ""
	audioTranscribeInput, audioTranscribeModel = "", ""
	return dir
}

// captureStdout runs fn with os.Stdout redirected to a pipe.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	runErr := fn()
	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("close pipe writer: %v", closeErr)
	}
	os.Stdout = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close pipe reader: %v", err)
	}
	return string(out), runErr
}

// assertDirEmpty fails when the local dry run wrote anything.
func assertDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read output dir: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("dry run wrote files: %v", names)
	}
}

func TestLocalSpeakDryRunDoesNotWriteFile(t *testing.T) {
	dir := withAudioDryRunFlags(t)
	audioSpeechInput = "hello world"

	out, err := captureStdout(t, func() error { return runLocalAudioSpeak(nil) })
	if err != nil {
		t.Fatalf("runLocalAudioSpeak() error = %v", err)
	}
	if !strings.Contains(out, "# audio speak dry run") {
		t.Errorf("output missing dry-run header:\n%s", out)
	}
	if !strings.Contains(out, "audio_<timestamp>.wav") {
		t.Errorf("output missing planned path:\n%s", out)
	}
	assertDirEmpty(t, dir)
}

func TestLocalTranscribeDryRunDoesNotWriteFile(t *testing.T) {
	dir := withAudioDryRunFlags(t)
	input := filepath.Join(t.TempDir(), "clip.wav")
	if err := os.WriteFile(input, []byte("not real audio"), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	audioTranscribeInput = input

	out, err := captureStdout(t, func() error { return runLocalAudioTranscribe(nil) })
	if err != nil {
		t.Fatalf("runLocalAudioTranscribe() error = %v", err)
	}
	if !strings.Contains(out, "# audio transcribe dry run") {
		t.Errorf("output missing dry-run header:\n%s", out)
	}
	if !strings.Contains(out, "clip.md") {
		t.Errorf("output missing planned transcription path:\n%s", out)
	}
	assertDirEmpty(t, dir)
}

func TestLocalSFXDryRunDoesNotWriteFile(t *testing.T) {
	dir := withAudioDryRunFlags(t)
	withSFXFlags(t)
	sfxDuration = 2

	p := &provider.EffectiveProvider{Type: types.ProviderLocal}
	out, err := captureStdout(t, func() error { return runLocalSFX("door slam", p, nil) })
	if err != nil {
		t.Fatalf("runLocalSFX() error = %v", err)
	}
	for _, want := range []string{"# audio sfx dry run", "stable-audio-open-small", "duration: 2.00s", "steps:    8"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	assertDirEmpty(t, dir)
}

func TestLocalSFXDryRunKeepsInjectValidation(t *testing.T) {
	withAudioDryRunFlags(t)
	withSFXFlags(t)

	p := &provider.EffectiveProvider{Type: types.ProviderLocal}
	_, err := captureStdout(t, func() error {
		return runLocalSFX("door slam", p, []byte(`{"unknown":1}`))
	})
	if err == nil {
		t.Fatal("runLocalSFX() with an unknown inject key = nil error, want validation error")
	}
}
