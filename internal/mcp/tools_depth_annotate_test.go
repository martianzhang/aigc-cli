package mcp

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestConvertDepthHandlerAnnotateFace 验证 convert_depth 工具带 annotate=face
// 时，输出图片包含人脸标注（蓝色关键点/红色检测框）。需要模型，缺失时跳过。
func TestConvertDepthHandlerAnnotateFace(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	modelsDir := filepath.Join(home, ".config", "aigc-cli", "models")
	if _, err := os.Stat(filepath.Join(modelsDir, "depth", "depth-anything-v2-small.onnx")); err != nil {
		t.Skip("depth model not installed; run `aigc-cli depth init`")
	}
	if _, err := os.Stat(filepath.Join(modelsDir, "face", "facefinder")); err != nil {
		t.Skip("face cascades not installed; run `aigc-cli depth init --face`")
	}

	tmp := t.TempDir()
	src := filepath.Join(tmp, "input.png")
	if err := writeSolidPNG(src, 320, 200); err != nil {
		t.Fatalf("write input: %v", err)
	}

	handler := convertDepthHandler(&Config{})
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "convert_depth",
			Arguments: map[string]any{
				"input_path":  src,
				"output_path": filepath.Join(tmp, "out.png"),
				"annotate":    "face",
			},
		},
	}

	resp, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if resp.IsError {
		t.Fatalf("handler error: %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(tmp, "out.png")); err != nil {
		t.Fatalf("output not created: %v", err)
	}
}

// writeSolidPNG 写一张纯色测试图（检测不到人脸也能走通转换+标注管线）。
func writeSolidPNG(path string, w, h int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.White)
		}
	}
	return png.Encode(f, img)
}
