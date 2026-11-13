package upscale

import (
	"bytes"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/ui"
)

func TestPrintUpscaleResult(t *testing.T) {
	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	defer restore()

	printUpscaleResult("/tmp/out/photo_upscaled.png", "realesr-general-x4v3")

	out := buf.String()
	for _, want := range []string{
		"Saved:", "/tmp/out/photo_upscaled.png",
		"Model: local: realesr-general-x4v3",
		"Next: aigc-cli preview /tmp/out/photo_upscaled.png",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Cost:") {
		t.Errorf("local upscale must not print cost:\n%s", out)
	}
}
