//go:build !cgo

package knowledge

import "fmt"

// ONNXEmbedder is not available without CGO.
type ONNXEmbedder struct{}

// NewONNXEmbedder returns an error when CGO is not available.
func NewONNXEmbedder(modelDir, libPath string) (*ONNXEmbedder, error) {
	return nil, fmt.Errorf("ONNX embedding requires CGO: rebuild with CGO_ENABLED=1")
}

// Embed is not supported without CGO.
func (e *ONNXEmbedder) Embed(text string) (Embedding, error) {
	return Embedding{}, fmt.Errorf("ONNX embedding requires CGO")
}

// Dim returns 0 for the stub.
func (e *ONNXEmbedder) Dim() int { return 0 }

// Close is a no-op for the stub.
func (e *ONNXEmbedder) Close() {}
