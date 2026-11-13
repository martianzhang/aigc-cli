package image

import (
	"bytes"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/testutil"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

func TestPrintImageSummary(t *testing.T) {
	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	defer restore()

	p := &provider.EffectiveProvider{
		Name:         "apimart",
		ProviderType: provider.APIMart,
		BaseURL:      "https://api.apimart.ai",
	}
	req := &types.GenerateRequest{Model: "gpt-image-2"}
	ctx := newImageDispatchCtx(p, req, false)
	a := testutil.AbsPath(t, "/tmp/a.png")
	b := testutil.AbsPath(t, "/tmp/b.png")
	printImageSummary(ctx, req, []string{a, b}, "task_1", 0.08)

	out := buf.String()
	for _, want := range []string{
		"Saved:",
		a,
		b,
		"Model: gpt-image-2 (apimart)",
		"Task: task_1",
		"Cost: $0.0800",
		"Next: aigc-cli task task_1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

func TestPrintImageSummaryLocal(t *testing.T) {
	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	defer restore()

	ctx := &imageDispatchCtx{p: &provider.EffectiveProvider{
		Type:         types.ProviderOllama,
		ProviderType: provider.OpenAI,
		BaseURL:      "http://127.0.0.1:11434/v1",
	}}
	req := &types.GenerateRequest{Model: "llava"}
	printImageSummary(ctx, req, []string{"/tmp/a.png"}, "", 0)

	out := buf.String()
	if !strings.Contains(out, "Model: local: llava") {
		t.Errorf("local model label missing:\n%s", out)
	}
	if strings.Contains(out, "Cost:") {
		t.Errorf("local summary must not print cost:\n%s", out)
	}
}

func TestPrintImageSummaryModelScopeUsesPreviewNotTask(t *testing.T) {
	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	defer restore()

	p := &provider.EffectiveProvider{
		ProviderType: provider.ModelScope,
		BaseURL:      "https://api-inference.modelscope.cn/v1",
	}
	req := &types.GenerateRequest{Model: "MiniMax/MiniMax-H3-Image"}
	ctx := newImageDispatchCtx(p, req, false)
	a := testutil.AbsPath(t, "/tmp/a.png")
	printImageSummary(ctx, req, []string{a}, "6aba753c-75e8-4568-ab3a-66935e7321f2", 0)

	out := buf.String()
	if strings.Contains(out, "aigc-cli task") {
		t.Errorf("ModelScope has no task command, got:\n%s", out)
	}
	if !strings.Contains(out, "Next: aigc-cli preview "+a) {
		t.Errorf("missing preview fallback:\n%s", out)
	}
	if !strings.Contains(out, "Task: 6aba753c-75e8-4568-ab3a-66935e7321f2") {
		t.Errorf("task id should still be visible:\n%s", out)
	}
}
