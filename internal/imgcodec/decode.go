package imgcodec

import (
	"fmt"
	"image"
	"io"
)

// WASM 编解码器可能「崩溃」而非返回错误：
//
// gen2brain 系列解码器（webp / avif / jpegxl）在 wazero 中运行 wasm，遇到
// 畸形输入时底层会触发 wasm trap，并以 Go panic（"unreachable"）的形式冒泡
// 出来。JPEG 曾是同一问题（jpegli 注册的 JPEG 解码器覆盖标准库），现已通过
// fork（github.com/martianzhang/jpegli）移除其 image.RegisterFormat 注册，
// JPEG 解码回落到标准库；其余 wasm 解码器仍需在解码边界恢复 panic。
//
// 任何需要解码非受信图片（下载、用户输入、MCP 传入）的调用点都应使用这里的
// Decode / DecodeConfig，而不是直接调用 image.Decode / image.DecodeConfig。

// Decode 等价于 image.Decode，但会恢复 wasm 解码器抛出的 panic，
// 将其转换为普通 error，避免畸形图片导致进程崩溃。
func Decode(r io.Reader) (img image.Image, format string, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			img, format, err = nil, "", fmt.Errorf("image decode failed: %v", rec)
		}
	}()
	return image.Decode(r)
}

// DecodeConfig 等价于 image.DecodeConfig，但会恢复 wasm 解码器抛出的 panic，
// 将其转换为普通 error，避免畸形图片导致进程崩溃。
func DecodeConfig(r io.Reader) (cfg image.Config, format string, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			cfg, format, err = image.Config{}, "", fmt.Errorf("image decode config failed: %v", rec)
		}
	}()
	return image.DecodeConfig(r)
}
