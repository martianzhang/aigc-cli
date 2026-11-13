package music

import (
	"bytes"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/testutil"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

func TestPrintMusicResultAPIMart(t *testing.T) {
	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	defer restore()

	req := &types.MusicGenerateRequest{Model: "suno"}
	f := testutil.AbsPath(t, "/tmp/out/music_task_1_0.mp3")
	printMusicResult(req, provider.APIMart, []string{f}, "task_1")

	out := buf.String()
	for _, want := range []string{
		"Saved:", f,
		"Model: suno (apimart)",
		"Task: task_1",
		"Next: aigc-cli task task_1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

func TestPrintMusicResultOpenRouterHasNoTaskCommand(t *testing.T) {
	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	defer restore()

	req := &types.MusicGenerateRequest{Model: "google/lyria-3"}
	f := testutil.AbsPath(t, "/tmp/out/music_1.mp3")
	printMusicResult(req, provider.OpenRouter, []string{f}, "")

	out := buf.String()
	if strings.Contains(out, "aigc-cli task") {
		t.Errorf("OpenRouter has no task command, got:\n%s", out)
	}
	if !strings.Contains(out, "Next: aigc-cli preview "+f) {
		t.Errorf("missing preview fallback:\n%s", out)
	}
}
