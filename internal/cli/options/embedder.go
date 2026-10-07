package options

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// Reserved embedding_provider values that select a built-in backend instead of
// a named provider from config.providers.
const (
	EmbeddingBackendLocal = "local" // ONNX if available, else hash
	EmbeddingBackendONNX  = "onnx"  // same as local
	EmbeddingBackendHash  = "hash"  // pure-Go n-gram hash
)

// BuildKBEmbedder builds the knowledge-base embedder from defaults.knowledgebase.
func BuildKBEmbedder() (knowledge.Embedder, error) {
	var ref, model string
	if cfg := KnowledgeDefaults(); cfg != nil {
		ref, model = cfg.EmbeddingProvider, cfg.EmbeddingModel
	}
	return BuildEmbedder(ref, model)
}

// BuildIdeasEmbedder builds the ideas embedder from defaults.ideas. It returns a
// nil embedder when no embedding_provider is configured, so ideas search stays
// keyword-only unless semantic retrieval is explicitly enabled.
func BuildIdeasEmbedder(ic *types.IdeasConfig) (knowledge.Embedder, error) {
	var ref, model string
	if ic != nil {
		ref, model = ic.EmbeddingProvider, ic.EmbeddingModel
	}
	if ref == "" {
		return nil, nil
	}
	return BuildEmbedder(ref, model)
}

// BuildEmbedder constructs the configured embedding backend.
//
// providerRef selects the backend:
//   - ""                     → local fallback (ONNX if available, else hash)
//   - "local" / "onnx"       → same local fallback (ONNX preferred)
//   - "hash"                 → pure-Go hash embedder
//   - any other name         → a provider in config.providers, called over the
//     OpenAI-compatible /v1/embeddings endpoint (Ollama or an online vendor)
//
// model is the embedding model id; it is required for a named provider.
func BuildEmbedder(providerRef, model string) (knowledge.Embedder, error) {
	modelsDir := filepath.Join(ConfigDir(), "models")
	switch providerRef {
	case "":
		return knowledge.NewEmbedder(nil, modelsDir, ONNXLibPath())
	case EmbeddingBackendLocal, EmbeddingBackendONNX:
		return knowledge.NewEmbedder(nil, modelsDir, ONNXLibPath())
	case EmbeddingBackendHash:
		return knowledge.NewHashEmbedder(384), nil
	}

	if model == "" {
		return nil, fmt.Errorf("embedding_provider %q requires embedding_model to be set", providerRef)
	}
	if err := provider.ValidateProviderRef(providerRef, providerMap(Shared.Cfg)); err != nil {
		return nil, err
	}
	ep := resolveNamedProvider(providerRef)
	cfg := &knowledge.EmbedConfig{BaseURL: ep.BaseURL, APIKey: ep.APIKey, Model: model}
	return knowledge.NewEmbedder(cfg, modelsDir, ONNXLibPath())
}

// resolveNamedProvider resolves a named provider reference to its effective
// base URL / API key, independent of any command's defaults.
func resolveNamedProvider(name string) *provider.EffectiveProvider {
	global := &provider.GlobalConfig{
		APIKey:  cfgString(Shared.Cfg, func(c *types.Config) string { return c.APIKey }),
		BaseURL: cfgString(Shared.Cfg, func(c *types.Config) string { return c.BaseURL }),
		Proxy:   cfgString(Shared.Cfg, func(c *types.Config) string { return c.HTTPProxy }),
	}
	return provider.ResolveCmdProvider(nil, name, providerMap(Shared.Cfg), global)
}

// ONNXLibPath returns the ONNX Runtime shared library path when present, else "".
func ONNXLibPath() string {
	var name string
	switch runtime.GOOS {
	case "darwin":
		name = "libonnxruntime.dylib"
	case "linux":
		name = "libonnxruntime.so"
	default:
		name = "onnxruntime.dll"
	}
	path := filepath.Join(ConfigDir(), "models", name)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}
