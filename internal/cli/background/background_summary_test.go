package background

import (
	"bytes"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

func TestPrintBackgroundResultLocal(t *testing.T) {
	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	defer restore()

	printBackgroundResult("/tmp/out/cat_removebg.png", nil)

	out := buf.String()
	for _, want := range []string{
		"Saved:", "/tmp/out/cat_removebg.png",
		"Model: local: rmbg-2.0",
		"Next: aigc-cli preview /tmp/out/cat_removebg.png",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

func TestPrintBackgroundResultOnline(t *testing.T) {
	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	defer restore()

	printBackgroundResult("/tmp/out/cat.png", &provider.EffectiveProvider{
		ProviderType: provider.APIMart,
		Model:        "gpt-image-2",
	})

	out := buf.String()
	if !strings.Contains(out, "Model: gpt-image-2 (apimart)") {
		t.Errorf("missing online model label:\n%s", out)
	}
}
