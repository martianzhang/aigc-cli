package hint

import (
	"errors"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
)

func TestErrorFormat(t *testing.T) {
	err := Error("model is required", "run 'aigc-cli models --type image --provider apimart'")
	if !strings.HasPrefix(err.Error(), "model is required\nHint: ") {
		t.Fatalf("Error() = %q", err)
	}
}

func TestWrapPreservesChain(t *testing.T) {
	base := errors.New("upstream failed")
	err := Wrap(base, "check --provider")
	if !errors.Is(err, base) {
		t.Fatal("Wrap() dropped the original error")
	}
	if !strings.Contains(err.Error(), "Hint: check --provider") {
		t.Fatalf("Wrap() = %q", err)
	}
}

func TestProviderName(t *testing.T) {
	p := &provider.EffectiveProvider{Name: "openrouter"}
	if got := ProviderName(p); got != "openrouter" {
		t.Fatalf("ProviderName() = %q, want openrouter", got)
	}
	if got := ProviderName(nil); got != "<name>" {
		t.Fatalf("ProviderName(nil) = %q", got)
	}
	if got := ProviderName(&provider.EffectiveProvider{ProviderType: provider.OpenAI}); got != "<name>" {
		t.Fatalf("ProviderName(unnamed) = %q, want <name>", got)
	}
}

func TestModelDiscoverIncludesProvider(t *testing.T) {
	p := &provider.EffectiveProvider{Name: "apimart"}
	want := "aigc-cli models --type image --provider apimart"
	if got := ModelDiscover("image", p); got != want {
		t.Fatalf("ModelDiscover() = %q, want %q", got, want)
	}
}

func TestMissingModelUsesProvider(t *testing.T) {
	p := &provider.EffectiveProvider{Name: "apimart"}
	err := MissingModel("image", p)
	if err == nil || !strings.Contains(err.Error(), "Hint: ") {
		t.Fatalf("MissingModel() = %v, want Hint", err)
	}
	if !strings.Contains(err.Error(), "aigc-cli models --type image --provider apimart") {
		t.Fatalf("MissingModel() = %q, want provider-specific discovery", err)
	}
}

func TestMissingLocalModelIsActionable(t *testing.T) {
	err := MissingLocalModel("upscale", "realesr-general-x4v3", "/tmp/models/x")
	if err == nil || !strings.Contains(err.Error(), "Hint: ") {
		t.Fatalf("MissingLocalModel() = %v, want Hint", err)
	}
	if !strings.Contains(err.Error(), "aigc-cli upscale init --model realesr-general-x4v3") {
		t.Fatalf("MissingLocalModel() = %q, want init command", err)
	}
}
