package cmd

import (
	"fmt"
	"image"

	"github.com/martianzhang/aigc-cli/internal/annotate"
)

// annotateSkeleton 检测人体骨架并叠加到深度图上（深度图 _depth.png）。
// 检测在输入原图上进行，骨架坐标与深度图同尺寸（深度图是原图尺寸）。
func annotateSkeleton(input, output string) error {
	if err := annotate.AnnotateImage(input, output, output, annotate.Options{Skeleton: true}); err != nil {
		return err
	}
	fmt.Printf("Skeleton annotated on depth: %s\n", output)
	return nil
}

// annotateFace 检测人脸（pigo 纯 Go）并叠加到深度图上。
func annotateFace(input, output string) error {
	if err := annotate.AnnotateImage(input, output, output, annotate.Options{Face: true}); err != nil {
		return err
	}
	fmt.Printf("Face annotated on depth: %s\n", output)
	return nil
}

// annotateVideoOptions 携带视频标注所需参数。
type annotateVideoOptions struct {
	skeleton bool
	face     bool
}

// newAnnotateVideoCallback 创建 depth.ConvertOptions.Annotate 回调。
// 每帧：加载原图帧 → 检测骨架/人脸 → 绘制到深度灰度帧 → 返回 RGBA 像素。
// 返回的 close 函数释放检测器资源。
func newAnnotateVideoCallback(opts annotateVideoOptions) (func(framePath string, gray *image.Gray) ([]uint8, bool), func(), error) {
	return annotate.AnnotateVideoCallback(annotate.Options{
		Skeleton: opts.skeleton,
		Face:     opts.face,
	})
}
