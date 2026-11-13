package audio

import (
	"bytes"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/ui"
)

func TestPrintAudioResultCloud(t *testing.T) {
	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	defer restore()

	printAudioResult([]string{"/tmp/out/speech_1.mp3"}, "gpt-4o-mini-tts", "openai")

	out := buf.String()
	for _, want := range []string{
		"Saved:", "/tmp/out/speech_1.mp3",
		"Model: gpt-4o-mini-tts (openai)",
		"Next: aigc-cli preview /tmp/out/speech_1.mp3",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

func TestPrintAudioResultLocal(t *testing.T) {
	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	defer restore()

	printAudioResult([]string{"/tmp/out/speech_1.wav"}, "local: kokoro", "")

	out := buf.String()
	if !strings.Contains(out, "Model: local: kokoro") {
		t.Errorf("missing local model label:\n%s", out)
	}
	if strings.Contains(out, "Cost:") {
		t.Errorf("local result must not print cost:\n%s", out)
	}
}
