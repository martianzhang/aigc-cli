package options

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestBuildEmbedderReservedBackends(t *testing.T) {
	orig := Shared.Cfg
	defer func() { Shared.Cfg = orig }()
	Shared.Cfg = nil

	for _, ref := range []string{"", "local", "onnx", "hash"} {
		e, err := BuildEmbedder(ref, "", 0)
		if err != nil {
			t.Errorf("BuildEmbedder(%q) error = %v", ref, err)
			continue
		}
		if e == nil || e.Dim() != 384 {
			t.Errorf("BuildEmbedder(%q) dim = %d, want 384", ref, e.Dim())
		}
	}
}

func TestBuildEmbedderNamedProvider(t *testing.T) {
	orig := Shared.Cfg
	defer func() { Shared.Cfg = orig }()

	Shared.Cfg = &types.Config{}
	if _, err := BuildEmbedder("nope", "m", 0); err == nil {
		t.Error("BuildEmbedder(unknown provider) = nil error, want error")
	}
	if _, err := BuildEmbedder("nope", "", 0); err == nil {
		t.Error("BuildEmbedder(missing model) = nil error, want error")
	}

	Shared.Cfg = &types.Config{Providers: map[string]*types.NamedProvider{
		"ollama": {Type: types.ProviderOllama, BaseURL: "http://localhost:11434"},
	}}
	e, err := BuildEmbedder("ollama", "embeddinggemma-2", 0)
	if err != nil {
		t.Fatalf("BuildEmbedder(ollama) error = %v", err)
	}
	if _, ok := e.(*knowledge.APIEmbedder); !ok {
		t.Errorf("BuildEmbedder(ollama) = %T, want *knowledge.APIEmbedder", e)
	}
}
