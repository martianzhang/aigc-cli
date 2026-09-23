package imgcodec

import (
	"fmt"
	"image"
	"io"
)

// WASM 编解码器可能「崩溃」而非返回错误：
//
// gen2brain 系列解码器（jpegli / webp / avif / jpegxl）在 wazero 中运行 wasm，
// 遇到畸形输入时底层会触发 wasm trap，并以 Go panic（"unreachable"）的形式
// 冒泡出来。jpegli 尤其明显——它的 decode() 只把自身的 procExit 哨兵转换为
// ErrDecode，其余 panic 一律 `panic(e)` 重新抛出（jpegli_wasm2go.go）。
//
// 更麻烦的是：image 包按注册顺序匹配格式，且只认第一个命中项，而
// "github.com/..." 在初始化顺序上早于 "image"，因此 jpegli 会抢先注册 JPEG
// 解码器（magic "\xff\xd8"）并覆盖标准库。结果是一张畸形 JPEG 就能让整个
// 进程 panic 崩溃。
//
// 由于 image.RegisterFormat 只能追加、无法注销，唯一的兜底方式是在解码边界
// 恢复 panic。任何需要解码非受信图片（下载、用户输入、MCP 传入）的调用点都应
// 使用这里的 Decode / DecodeConfig，而不是直接调用 image.Decode /
// image.DecodeConfig。

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
