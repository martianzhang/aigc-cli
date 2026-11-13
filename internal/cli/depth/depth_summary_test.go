package depth

import (
	"bytes"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/testutil"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

func TestPrintDepthResult(t *testing.T) {
	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	defer restore()

	f := testutil.AbsPath(t, "/tmp/out/photo_depth.png")
	printDepthResult(f, "depth-anything-v2-small")

	out := buf.String()
	for _, want := range []string{
		"Saved:", f,
		"Model: local: depth-anything-v2-small",
		"Next: aigc-cli preview " + f,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}
