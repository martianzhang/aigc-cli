package knowledge

// NewEmbedder selects the embedding backend. When cfg is nil or lacks a base
// URL or model, it falls back to the best local option (ONNX when available,
// else the pure-Go hash embedder).
func NewEmbedder(cfg *EmbedConfig, modelsDir, onnxLibPath string) (Embedder, error) {
	if cfg == nil || cfg.BaseURL == "" || cfg.Model == "" {
		return BestEmbedder(modelsDir, onnxLibPath), nil
	}
	return NewAPIEmbedder(*cfg), nil
}
