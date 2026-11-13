package video

import (
	"bytes"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/testutil"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

func TestPrintVideoResult(t *testing.T) {
	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	defer restore()

	req := &types.VideoGenerateRequest{Model: "veo-3.1"}
	p := &provider.EffectiveProvider{ProviderType: provider.APIMart}
	f := testutil.AbsPath(t, "/tmp/out/video_task_1_0.mp4")
	printVideoResult(req, p, []string{f}, "task_1")

	out := buf.String()
	for _, want := range []string{
		"Saved:", f,
		"Model: veo-3.1 (apimart)",
		"Task: task_1",
		"Next: aigc-cli task task_1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

func TestPrintVideoResultOpenRouterUsesJobID(t *testing.T) {
	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	defer restore()

	req := &types.VideoGenerateRequest{Model: "google/veo-3.1"}
	p := &provider.EffectiveProvider{ProviderType: provider.OpenRouter}
	printVideoResult(req, p, []string{"/tmp/out/video_job_1_0.mp4"}, "job_1")

	out := buf.String()
	if !strings.Contains(out, "Next: aigc-cli video --job-id job_1") {
		t.Errorf("missing job-id resume:\n%s", out)
	}
}
