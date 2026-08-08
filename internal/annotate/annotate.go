// Package annotate 提供骨架/人脸标注的共享逻辑，供 cmd（depth 命令）与
// internal/mcp（convert_depth 工具）复用。图片标注为一次性操作，视频标注
// 以回调形式注入 depth.ConvertOptions.Annotate，实现逐帧叠加。
package annotate

import (
	"fmt"
	"image"
	"os"
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/face"
	"github.com/martianzhang/aigc-cli/internal/skeleton"
)

// Options 控制标注内容。
type Options struct {
	Skeleton bool
	Face     bool
}

// ModelsDir 返回模型根目录（~/.config/aigc-cli/models）。
func ModelsDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".config", "aigc-cli", "models")
	}
	return filepath.Join(home, ".config", "aigc-cli", "models")
}

// defaultLibPath 返回 ONNX Runtime 库路径。
func defaultLibPath(modelsDir string) (string, error) {
	for _, name := range []string{"libonnxruntime.dylib", "libonnxruntime.so", "onnxruntime.dll"} {
		p := filepath.Join(modelsDir, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("onnx runtime library not found in %s", modelsDir)
}

// annotators 持有已加载的检测器（skeleton + face）。
type annotators struct {
	skel  *skeleton.Detector
	face  *face.Detector
	close []func()
}

// load 按选项加载检测器。
func load(opts Options) (*annotators, error) {
	a := &annotators{}
	modelsDir := ModelsDir()

	if opts.Skeleton {
		libPath, err := defaultLibPath(modelsDir)
		if err != nil {
			return nil, err
		}
		modelPath := filepath.Join(modelsDir, "skeleton", "yolov8n-pose.onnx")
		if _, err := os.Stat(modelPath); err != nil {
			return nil, fmt.Errorf("skeleton model not found at %s\n  Run: aigc-cli depth init --skeleton", modelPath)
		}
		det, err := skeleton.NewDetector(libPath, modelPath)
		if err != nil {
			return nil, fmt.Errorf("init skeleton detector: %w", err)
		}
		a.skel = det
		a.close = append(a.close, det.Close)
	}

	if opts.Face {
		det, err := face.NewDetector(modelsDir)
		if err != nil {
			a.Close()
			return nil, fmt.Errorf("init face detector: %w", err)
		}
		a.face = det
	}

	return a, nil
}

// Close 释放检测器资源（face 为纯 Go pigo，无需释放）。
func (a *annotators) Close() {
	for i := len(a.close) - 1; i >= 0; i-- {
		a.close[i]()
	}
}

// annotate 把原图上的检测结果绘制到 dst 上。
func (a *annotators) annotate(dst *image.RGBA, src image.Image) {
	if a.skel != nil {
		if people, err := a.skel.Detect(src, 0.5); err == nil {
			skeleton.DrawSkeleton(dst, people)
		}
	}
	if a.face != nil {
		if faces, err := a.face.Detect(src); err == nil {
			face.Draw(dst, faces)
		}
	}
}

// toRGBA 转换为 RGBA（便于绘制）。
func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok {
		return r
	}
	b := img.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(x, y, img.At(x, y))
		}
	}
	return dst
}

// LoadImage 解码图片文件。
func LoadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}
