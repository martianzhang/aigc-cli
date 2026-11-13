package depth

import (
	"bytes"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/ui"
)

func TestPrintDepthResult(t *testing.T) {
	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	defer restore()

	printDepthResult("/tmp/out/photo_depth.png", "depth-anything-v2-small")

	out := buf.String()
	for _, want := range []string{
		"Saved:", "/tmp/out/photo_depth.png",
		"Model: local: depth-anything-v2-small",
		"Next: aigc-cli preview /tmp/out/photo_depth.png",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}
