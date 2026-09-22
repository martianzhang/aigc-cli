package ideas

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/ideas"
)

func TestSourceFlagRegistration(t *testing.T) {
	cmd := NewCommand(func() Deps { return Deps{} })

	fl := cmd.Flags().Lookup("source")
	if fl == nil {
		t.Fatal("--source flag is not registered")
	}
	if fl.DefValue != "[all]" {
		t.Errorf("--source default = %q, want %q", fl.DefValue, "[all]")
	}
	for _, name := range ideas.AllSourceNames() {
		if !strings.Contains(fl.Usage, name) {
			t.Errorf("--source usage %q does not list %q", fl.Usage, name)
		}
	}

	if !strings.Contains(cmd.Long, "Sources (--source)") {
		t.Error("Long text does not document the sources section")
	}
	for _, name := range ideas.AllSourceNames() {
		if !strings.Contains(cmd.Long, name) {
			t.Errorf("Long text does not list source %q", name)
		}
	}
	if !strings.Contains(cmd.Example, "--source aipromptslibrary") {
		t.Errorf("Example does not show an online source: %q", cmd.Example)
	}
}
