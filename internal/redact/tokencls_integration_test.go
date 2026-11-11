package redact

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// TestTokenClassifierIntegration uses a tiny ONNX fixture (constant graph) to
// exercise real session creation, sliding windows and BIO decoding. It skips
// when the local ONNX Runtime library is not installed.
func TestTokenClassifierIntegration(t *testing.T) {
	if os.Getenv("AIGC_REDACT_ONNX_TEST") == "" {
		t.Skip("set AIGC_REDACT_ONNX_TEST=1 to run the ONNX integration test")
	}
	modelsDir := filepath.Join(os.Getenv("HOME"), ".config", "aigc-cli", "models")
	if _, err := onnxrt.LibPath(modelsDir); err != nil {
		t.Skip("onnxruntime not installed:", err)
	}
	dir, err := filepath.Abs(filepath.Join("testdata", "tokencls"))
	if err != nil {
		t.Fatal(err)
	}
	tc, err := NewTokenClassifier(&types.RedactTokenclsConfig{ModelDir: dir, MaxLen: 512, Stride: 128}, modelsDir)
	if err != nil {
		t.Fatal(err)
	}
	defer tc.Close()
	for _, text := range []string{"电话 13812345678"} {
		result, err := tc.Detect(context.Background(), NewDocument("t", text))
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Spans) == 0 {
			t.Fatalf("expected the tiny fixture to emit a PHONE span for %q", text)
		}
	}
}
