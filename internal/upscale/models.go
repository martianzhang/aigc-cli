package upscale

import (
	"fmt"
	"path/filepath"
)

// modelsBaseURL is the shared release that hosts every local model file,
// matching the convention used by vision/ocr/audio/background.
const modelsBaseURL = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1"

// ModelInfo describes one downloadable super-resolution model. File is the
// local filename (the release asset is "upscale-"+File); License is the SPDX id
// of the upstream weights; Source is the upstream project, for attribution.
type ModelInfo struct {
	ID      string
	Name    string
	File    string
	Scale   int
	SizeMB  float32
	License string
	Source  string
	InName  string
	OutName string
}

// Models is the curated registry. Every entry is hosted on aigc-cli-models and
// has been verified to download and run through this package.
//
// Licenses are those of the *upstream* weights (the algorithms), not of the
// third-party ONNX re-exports we fetched them from:
//   - Real-ESRGAN (xinntao/Real-ESRGAN) → BSD-3-Clause
//   - Real-CUGAN  (bilibili/ailab)       → MIT
//   - Swin2SR     (mv-lab/swin2sr)       → Apache-2.0
var Models = []ModelInfo{
	{ID: "realesr-general-x4v3", Name: "Real-ESRGAN General x4v3", File: "realesr-general-x4v3.onnx", Scale: 4, SizeMB: 4.9, License: "BSD-3-Clause", Source: "xinntao/Real-ESRGAN"},
	{ID: "real-esrgan-x4plus", Name: "Real-ESRGAN x4plus", File: "real-esrgan-x4plus.onnx", Scale: 4, SizeMB: 67, License: "BSD-3-Clause", Source: "xinntao/Real-ESRGAN"},
	{ID: "real-esrgan-x4plus-anime-6b", Name: "Real-ESRGAN x4plus Anime 6B", File: "real-esrgan-x4plus-anime-6b.onnx", Scale: 4, SizeMB: 18, License: "BSD-3-Clause", Source: "xinntao/Real-ESRGAN", InName: "image.1", OutName: "image"},
	{ID: "real-esrgan-animevideov3", Name: "Real-ESRGAN AnimeVideo v3", File: "real-esrgan-animevideov3.onnx", Scale: 4, SizeMB: 2.5, License: "BSD-3-Clause", Source: "xinntao/Real-ESRGAN"},
	{ID: "real-cugan-2x", Name: "Real-CUGAN 2x (HFA2k)", File: "real-cugan-2x.onnx", Scale: 2, SizeMB: 5.2, License: "MIT", Source: "bilibili/ailab (Real-CUGAN)"},
	{ID: "swin2sr-lightweight-x2", Name: "Swin2SR Lightweight x2", File: "swin2sr-lightweight-x2.onnx", Scale: 2, SizeMB: 8.1, License: "Apache-2.0", Source: "mv-lab/swin2sr", InName: "pixel_values", OutName: "reconstruction"},
	{ID: "swin2sr-realworld-x4", Name: "Swin2SR RealWorld x4", File: "swin2sr-realworld-x4.onnx", Scale: 4, SizeMB: 53, License: "Apache-2.0", Source: "mv-lab/swin2sr", InName: "pixel_values", OutName: "reconstruction"},
}

// DefaultModelID is the model used when --model is not given.
const DefaultModelID = "realesr-general-x4v3"

// ResolveModel returns the ModelInfo for id, or the default when id is empty.
func ResolveModel(id string) (ModelInfo, bool) {
	if id == "" {
		id = DefaultModelID
	}
	for _, m := range Models {
		if m.ID == id {
			return m, true
		}
	}
	return ModelInfo{}, false
}

// Lookup returns the ModelInfo for an exact id (no defaulting).
func Lookup(id string) (ModelInfo, error) {
	for _, m := range Models {
		if m.ID == id {
			return m, nil
		}
	}
	return ModelInfo{}, fmt.Errorf("unknown model %q; available: %v", id, ModelIDs())
}

// ModelIDs returns all known model ids in registry order.
func ModelIDs() []string {
	ids := make([]string, 0, len(Models))
	for _, m := range Models {
		ids = append(ids, m.ID)
	}
	return ids
}

// Dir returns the directory where upscale models live.
func Dir(modelsDir string) string { return filepath.Join(modelsDir, "upscale") }

// ModelPath resolves the on-disk path of a model under the given models root.
func ModelPath(modelsDir, id string) string {
	m, ok := ResolveModel(id)
	if !ok {
		return filepath.Join(Dir(modelsDir), id)
	}
	return filepath.Join(Dir(modelsDir), m.File)
}

// URL returns the release download URL for a model.
func URL(m ModelInfo) string { return modelsBaseURL + "/upscale-" + m.File }

// Input returns the model's input tensor name (default InputName).
func (m ModelInfo) Input() string {
	if m.InName != "" {
		return m.InName
	}
	return InputName
}

// Output returns the model's output tensor name (default OutputName).
func (m ModelInfo) Output() string {
	if m.OutName != "" {
		return m.OutName
	}
	return OutputName
}
