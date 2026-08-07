package depth

import "path/filepath"

// ModelInfo 描述一个 Depth Anything V2 变体。
type ModelInfo struct {
	// ID 是完整的模型标识（如 depth-anything-v2-small），用于 --depth-model。
	ID string
	// Alias 是便捷别名（如 small），与 ID 一一对应。
	Alias string
	// Filename 是模型文件在 models 目录下的文件名。
	Filename string
	// Desc 是人类可读描述（用于 init 命令打印和 --dry-run）。
	Desc string
	// Size 是模型文件大小的人类可读描述。
	Size string
	// License 是模型许可证（显示给用户，Base/Large 禁止商用）。
	License string
}

// Models 是深度估计模型注册表。
//
// 许可证差异：
//   - depth-anything-v2-small: Apache-2.0（商用友好，默认）
//   - depth-anything-v2-base/large: CC-BY-NC-4.0（非商业用途）
var Models = map[string]ModelInfo{
	"depth-anything-v2-small": {
		ID:       "depth-anything-v2-small",
		Alias:    "small",
		Filename: "depth-anything-v2-small.onnx",
		Desc:     "Depth Anything V2 Small (DINOv2 ViT-S), 24.8M params",
		Size:     "99MB",
		License:  "Apache-2.0",
	},
	"depth-anything-v2-base": {
		ID:       "depth-anything-v2-base",
		Alias:    "base",
		Filename: "depth-anything-v2-base.onnx",
		Desc:     "Depth Anything V2 Base (DINOv2 ViT-B), 97.5M params",
		Size:     "370MB",
		License:  "CC-BY-NC-4.0",
	},
	"depth-anything-v2-large": {
		ID:       "depth-anything-v2-large",
		Alias:    "large",
		Filename: "depth-anything-v2-large.onnx",
		Desc:     "Depth Anything V2 Large (DINOv2 ViT-L), 335M params",
		Size:     "1.25GB",
		License:  "CC-BY-NC-4.0",
	},
}

// DefaultModelID 返回默认模型 ID（Apache-2.0 商用友好）。
const DefaultModelID = "depth-anything-v2-small"

// ResolveModel 解析模型标识（支持全名或别名），返回对应 ModelInfo。
// 未知标识返回 (zero value, false)。
func ResolveModel(id string) (ModelInfo, bool) {
	if m, ok := Models[id]; ok {
		return m, true
	}
	for _, m := range Models {
		if m.Alias == id {
			return m, true
		}
	}
	return ModelInfo{}, false
}

// ModelPath 返回指定模型的完整路径；未知标识回退到默认模型。
func ModelPath(modelsDir, id string) string {
	m, ok := ResolveModel(id)
	if !ok {
		m = Models[DefaultModelID]
	}
	return filepath.Join(modelsDir, "depth", m.Filename)
}

// ListModelIDs 返回全部模型 ID（按字母序），用于 --help 展示。
func ListModelIDs() []string {
	return []string{DefaultModelID, "depth-anything-v2-base", "depth-anything-v2-large"}
}
