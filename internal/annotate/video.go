package annotate

import (
	"fmt"
	"image"
	"image/png"
	"os"
)

// AnnotateImage 在已生成的深度图上叠加骨架/人脸标注并保存。
// input 是原始图像（用于检测），depthPath 是深度图路径，output 是输出路径。
func AnnotateImage(input, depthPath, output string, opts Options) error {
	det, err := load(opts)
	if err != nil {
		return err
	}
	defer det.Close()

	src, err := LoadImage(input)
	if err != nil {
		return err
	}
	depthImg, err := LoadImage(depthPath)
	if err != nil {
		return fmt.Errorf("load depth image %s: %w\nRun depth conversion first", depthPath, err)
	}

	dst := toRGBA(depthImg)
	det.annotate(dst, src)

	f, err := os.Create(output)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, dst)
}

// AnnotateVideoCallback 创建逐帧标注回调 + 资源释放函数。
// 回调语义与 depth.ConvertOptions.Annotate 一致：framePath 此时仍是原图，
// gray 是归一化深度灰度帧；返回 (RGBA 像素, true) 表示用标注帧覆盖输出。
func AnnotateVideoCallback(opts Options) (func(framePath string, gray *image.Gray) ([]uint8, bool), func(), error) {
	det, err := load(opts)
	if err != nil {
		return nil, nil, err
	}

	cb := func(framePath string, gray *image.Gray) ([]uint8, bool) {
		src, err := LoadImage(framePath)
		if err != nil {
			return nil, false
		}
		dst := toRGBA(gray)
		det.annotate(dst, src)
		return dst.Pix, true
	}
	return cb, det.Close, nil
}
