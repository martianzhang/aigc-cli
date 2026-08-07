// Package imgcodec 提供基于 wasm codec（gen2brain 系列）的图片编解码增强。
//
// 解码：import 本包即通过 image.RegisterFormat 注册 AVIF / WebP / JPEG-XL
//
//	解码器（HEIC/HEIF 由 libavif 的 HEIF 容器能力顺带支持），此后
//	image.Decode 全链路自动支持这些格式，无需改动任何调用点。
//
// 编码：提供统一 EncodeToFile 入口，内部按格式分发到 libavif / libwebp /
//
//	jpegli / libjxl 的 wasm 实现（零 CGO）。相比 Go 标准库编码器，
//	同质量下 JPEG/WebP 体积更小，且新增 AVIF / JXL 两种高压缩率格式。
//
// 运行时：wasm 模块通过 go:embed 打包在 gen2brain 库内，首次调用时惰性
//
//	编译（wazero），可用 Init() 在启动时预编译避免首调用延迟。
package imgcodec

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"

	"github.com/gen2brain/avif"
	"github.com/gen2brain/jpegli"
	"github.com/gen2brain/jpegxl"
	"github.com/gen2brain/webp"
)

func init() {
	// gen2brain 库的 init() 已自动注册：
	//   avif:  "????ftypavif" / "????ftypavis"
	//   webp:  "RIFF????WEBPVP8"
	//   jpegxl: "????JXL" / "\xff\x0a"
	//
	// HEIC/HEIF 与 AVIF 同为 HEIF 容器（libavif 底层 libheif 可解析），
	// 显式注册常见 HEIF brand，使 image.Decode 也能读 HEIC 手机图片。
	// ftyp box 的 size 字段随文件不同，注册常见取值；匹配不上时
	// 仍有 avif 的 "????ftypavif" 兜底（AVIF 文件必然命中）。
	for _, magic := range []string{
		"\x00\x00\x00\x18ftypheic",
		"\x00\x00\x00\x18ftypheix",
		"\x00\x00\x00\x18ftypmif1",
		"\x00\x00\x00\x18ftypmsf1",
	} {
		image.RegisterFormat("heic", magic, decodeHEIC, avif.DecodeConfig)
	}
}

// decodeHEIC adapts avif.Decode (variadic options) to the image.Decoder
// signature required by image.RegisterFormat.
func decodeHEIC(r io.Reader) (image.Image, error) {
	return avif.Decode(r)
}

// Init 预编译 avif / jpegli / jpegxl 的 wasm 模块，避免首次调用延迟。
// 建议在启动入口（PersistentPreRunE）调用一次。
func Init() {
	jpegli.Init()
	jpegxl.InitDecoder()
	jpegxl.InitEncoder()
	// avif 无公开 Init，首次调用时惰性编译
}

// SniffImageExt 通过魔数嗅探图片真实格式，返回小写扩展名（含点号），
// 无法识别时返回空字符串。
//
// 用于修正下载保存时的扩展名：API 返回的 URL 常缺扩展名或带 query 参数，
// 用 URL 猜格式不可靠（无扩展名默认 .png 但实际可能是 webp）。
func SniffImageExt(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		return ".jpg"
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return ".png"
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return ".gif"
	case bytes.HasPrefix(data, []byte("BM")):
		return ".bmp"
	case bytes.HasPrefix(data, []byte("RIFF")) && len(data) >= 12 && string(data[8:12]) == "WEBP":
		return ".webp"
	case len(data) >= 12 && string(data[4:8]) == "ftyp":
		// HEIF 家族：根据 brand 区分 AVIF 与 HEIC
		switch string(data[8:12]) {
		case "avif", "avis":
			return ".avif"
		case "heic", "heix", "hevc", "hevx", "mif1", "msf1":
			return ".heic"
		default:
			return ".heif"
		}
	case bytes.HasPrefix(data, []byte{0xff, 0x0a}):
		return ".jxl"
	}
	return ""
}

// Encode 将图片按 format 编码到 w，返回写入字节数。
// 支持格式：jpg/jpeg（jpegli）、webp（libwebp）、avif（libavif）、
// jxl（libjxl）、png（标准库无损）。
func Encode(w io.Writer, m image.Image, format string, quality int) (int64, error) {
	if format == "png" {
		// PNG 无损：quality 参数无效，走标准库
		cw := &countingWriter{w: w}
		if err := png.Encode(cw, m); err != nil {
			return 0, fmt.Errorf("png encode: %w", err)
		}
		return cw.n, nil
	}

	buf := &bytes.Buffer{}
	switch format {
	case "jpg", "jpeg":
		if err := jpegli.Encode(buf, m, &jpegli.EncodingOptions{Quality: quality}); err != nil {
			return 0, fmt.Errorf("jpegli encode: %w", err)
		}
	case "webp":
		if err := webp.Encode(buf, m, webp.Options{Quality: quality}); err != nil {
			return 0, fmt.Errorf("webp encode: %w", err)
		}
	case "avif":
		if err := avif.Encode(buf, m, avif.Options{Quality: quality}); err != nil {
			return 0, fmt.Errorf("avif encode: %w", err)
		}
	case "jxl":
		if err := jpegxl.Encode(buf, m, jpegxl.Options{Quality: quality}); err != nil {
			return 0, fmt.Errorf("jxl encode: %w", err)
		}
	default:
		return 0, fmt.Errorf("unsupported output format: %s", format)
	}

	if _, err := w.Write(buf.Bytes()); err != nil {
		return 0, err
	}
	return int64(buf.Len()), nil
}

// EncodeToFile 将图片按 format 编码到 path，返回文件字节数。
func EncodeToFile(m image.Image, path, format string, quality int) (int64, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	n, err := Encode(f, m, format, quality)
	if err != nil {
		return 0, err
	}
	return n, nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	cw.n += int64(n)
	return n, err
}
