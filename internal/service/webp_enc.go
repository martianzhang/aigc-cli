package service

import (
	"image"
	"io"

	"github.com/martianzhang/aigc-cli/internal/imgcodec"
)

// webpEncode 编码 WebP 图像。实现基于 gen2brain/webp（libwebp wasm 转译，
// 纯 Go、无 CGO），替代了早期依赖 chai2010/webp 的 CGO 实现。
func webpEncode(w io.Writer, m image.Image, quality int) error {
	_, err := imgcodec.Encode(w, m, "webp", quality)
	return err
}
