package service

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"html"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mattn/go-sixel"
	_ "golang.org/x/image/webp"

	"github.com/richardwooding/c2pa"
)

// PreviewFile opens a file or URL with the system default application.
// For image files, it also attempts inline terminal display when the
// terminal supports it (Kitty, iTerm2, or Sixel).
// URLs are downloaded to a temporary file first.
func PreviewFile(path string) error {
	// Detect URL — download to temp file first
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		data, err := FetchImage(path)
		if err != nil {
			return fmt.Errorf("failed to download %s: %w", path, err)
		}
		// Detect image extension from magic bytes
		ext := detectImageExt(data)
		// Generate a filename from the URL
		saveName := urlToFilename(path, ext)
		if err := os.WriteFile(saveName, data, 0644); err != nil {
			return fmt.Errorf("failed to save %s: %w", saveName, err)
		}
		fmt.Printf("Saved: %s\n", saveName)
		path = saveName
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}

	// Try inline terminal image display for supported terminals
	if isImageFile(path) {
		if trySixelImage(path) {
			return nil
		}
		if tryInlineImage(path) {
			return nil
		}
	}

	return openSystemDefault(path)
}

// openSystemDefault opens the file with the operating system's default handler.
func openSystemDefault(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open %s: %w", path, err)
	}
	return nil
}

// trySixelImage attempts to display an image inline using the Sixel protocol.
// Returns true if successful (terminal supports sixel and encoding succeeded).
func trySixelImage(path string) bool {
	if !isSixelCapableTerminal() {
		return false
	}

	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return false
	}

	// Get terminal width for sizing
	termWidth := 80
	imgBounds := img.Bounds()
	imgW := imgBounds.Dx()
	imgH := imgBounds.Dy()

	// Scale to fit terminal width while maintaining aspect ratio
	if imgW > termWidth*8 { // sixel uses 8 pixels per terminal column
		scale := float64(termWidth*8) / float64(imgW)
		newW := int(float64(imgW) * scale)
		newH := int(float64(imgH) * scale)
		img = resizeImage(img, newW, newH)
	}

	enc := sixel.NewEncoder(os.Stdout)
	enc.Width = img.Bounds().Dx()
	enc.Height = img.Bounds().Dy()

	if err := enc.Encode(img); err != nil {
		return false
	}
	return true
}

// resizeImage scales an image to the given dimensions using nearest-neighbor.
func resizeImage(src image.Image, width, height int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			sx := x * srcW / width
			sy := y * srcH / height
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

// tryInlineImage attempts to display an image inline in the terminal
// using the iTerm2 inline image protocol. Returns true if successful.
func tryInlineImage(path string) bool {
	if !isInlineCapableTerminal() {
		return false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	ext := strings.ToLower(filepath.Ext(path))
	mime := mimeFromExt(ext)
	if mime == "" {
		return false
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	fmt.Printf("\033]1337;File=inline=1;preserveAspectRatio=1;mimeType=%s:%s\a\n", mime, encoded)
	return true
}

// isSixelCapableTerminal checks if the current terminal supports Sixel.
func isSixelCapableTerminal() bool {
	// WezTerm
	if os.Getenv("TERM_PROGRAM") == "WezTerm" {
		return true
	}
	// mintty (Git Bash on Windows)
	if os.Getenv("TERM_PROGRAM") == "mintty" {
		return true
	}
	// xterm with explicit sixel support
	if strings.Contains(os.Getenv("TERM"), "sixel") {
		return true
	}
	// Windows Terminal (has WT_SESSION, supports sixel since v1.22+)
	if os.Getenv("WT_SESSION") != "" {
		return true
	}
	// foot terminal
	if os.Getenv("TERM") == "foot" {
		return true
	}
	return false
}

// isInlineCapableTerminal checks if the current terminal supports iTerm2/Kitty inline images.
func isInlineCapableTerminal() bool {
	// iTerm2 sets TERM_PROGRAM=iTerm.app
	if os.Getenv("TERM_PROGRAM") == "iTerm.app" {
		return true
	}
	// Kitty sets KITTY_WINDOW_ID
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return true
	}
	return false
}

// detectImageExt guesses the file extension from image magic bytes.
func detectImageExt(data []byte) string {
	if len(data) < 4 {
		return ".bin"
	}
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return ".png"
	}
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return ".jpg"
	}
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 {
		return ".gif"
	}
	if data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 {
		return ".webp"
	}
	return ".bin"
}

// urlToFilename extracts a filename from a URL, falling back to a timestamp if
// the URL path doesn't have a usable name. Ensures the file extension matches.
func urlToFilename(rawURL, ext string) string {
	// Strip query params
	if idx := strings.Index(rawURL, "?"); idx >= 0 {
		rawURL = rawURL[:idx]
	}
	base := filepath.Base(rawURL)
	// Remove existing extension
	if e := filepath.Ext(base); e != "" {
		base = base[:len(base)-len(e)]
	}
	// If the base is empty or generic ("download", "image"), use a hash
	if base == "" || base == "." || base == "/" {
		base = fmt.Sprintf("image_%d", time.Now().Unix())
	}
	// Ensure unique name
	name := base + ext
	if _, err := os.Stat(name); err == nil {
		for i := 1; ; i++ {
			name = fmt.Sprintf("%s_%d%s", base, i, ext)
			if _, err := os.Stat(name); os.IsNotExist(err) {
				break
			}
		}
	}
	return name
}

// PrintImageInfo writes image metadata (file stats, dimensions, embedded metadata)
// to w. It extracts PNG text chunks, JPEG comments, and EXIF software tags.
// Non-fatal errors (e.g. corrupt metadata) are silently ignored.
func PrintImageInfo(w io.Writer, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "━━━ %s ━━━\n", filepath.Base(path))
	fmt.Fprintf(w, "  Size:     %s\n", humanSize(info.Size()))
	fmt.Fprintf(w, "  Modified: %s\n", info.ModTime().Format("2006-01-02 15:04:05"))

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	config, format, err := image.DecodeConfig(f)
	if err != nil {
		fmt.Fprintf(w, "  (not a recognized image format)\n")
		return nil
	}
	fmt.Fprintf(w, "  Format:   %s\n", strings.ToUpper(format))
	fmt.Fprintf(w, "  Dims:     %d × %d\n", config.Width, config.Height)

	// C2PA watermark detection via library (PNG & JPEG)
	f.Seek(0, io.SeekStart)
	var c2paInfo c2pa.Info
	hasC2PA := false
	var c2paVendor, c2paSw, c2paVer, c2paSource string

	// TC260 AIGC label (China GB 45438-2025)
	hasTC260 := false
	var tc260data string

	switch format {
	case "png":
		c2paInfo = c2pa.Read(context.Background(), c2pa.PNG, f)
	case "jpeg":
		c2paInfo = c2pa.Read(context.Background(), c2pa.JPEG, f)
	}
	if c2paInfo.Present {
		hasC2PA = true
		c2paVendor = c2paInfo.SignedBy
		c2paSw = c2paInfo.Title
		if c2paInfo.AIGenerated {
			c2paSource = "AI Generated"
		}
	}

	// Format-specific metadata extraction and C2PA fallback
	f.Seek(0, io.SeekStart)
	switch format {
	case "png":
		textChunks, rawC2PA := readPNGChunks(f)

		// Fallback: library missed it but raw caBX chunk exists
		if !hasC2PA {
			if v, sw, ver, src := extractC2PAInfo(rawC2PA); v != "" || sw != "" {
				hasC2PA = true
				c2paVendor, c2paSw, c2paVer, c2paSource = v, sw, ver, src
			}
		}

		// Print watermark info (C2PA / SynthID)
		if hasC2PA {
			printC2PAInfo(w, c2paVendor, c2paSw, c2paVer, c2paSource)
		}

		// China TC260 AIGC label (GB 45438-2025)
		if tc260, ok := textChunks["TC260"]; ok {
			hasTC260 = true
			tc260data = tc260
			delete(textChunks, "TC260")
		}
		// Also check XMP metadata for TC260:AIGC
		if !hasTC260 {
			if xmp, ok := textChunks["XML:com.adobe.xmp"]; ok {
				if idx := strings.Index(xmp, "<TC260:AIGC>"); idx >= 0 {
					end := strings.Index(xmp[idx:], "</TC260:AIGC>")
					if end > 0 {
						hasTC260 = true
						tc260data = xmp[idx+12 : idx+end]
					}
				}
			}
		}
		if hasTC260 {
			printTC260(w, tc260data)
		}

		// PNG text chunks (skip internal XMP metadata)
		delete(textChunks, "XML:com.adobe.xmp")
		if len(textChunks) > 0 {
			fmt.Fprintf(w, "  Metadata:\n")
			for _, key := range []string{"Software", "Description", "Generation", "parameters", "Title"} {
				if v, ok := textChunks[key]; ok {
					fmt.Fprintf(w, "    %s: %s\n", key, truncateMeta(v))
					delete(textChunks, key)
				}
			}
			for k, v := range textChunks {
				fmt.Fprintf(w, "    %s: %s\n", k, truncateMeta(v))
			}
		}

	case "jpeg":
		jf := readJPEGInfo(f)
		if !hasC2PA && jf.hasC2PA {
			hasC2PA = true
			c2paSource = "C2PA data detected (library parse failed)"
		}
		if hasC2PA {
			printC2PAInfo(w, c2paVendor, c2paSw, c2paVer, c2paSource)
		}
		if jf.comment != "" {
			fmt.Fprintf(w, "  Comment:  %s\n", truncateMeta(jf.comment))
		}
		// TC260 AIGC label in EXIF (JPEG stores it in APP1 JSON)
		if !hasTC260 && jf.tc260Data != "" {
			tc260data = jf.tc260Data
			hasTC260 = true
		}
		if hasTC260 {
			printTC260(w, tc260data)
		}

	default:
		// WebP, GIF, BMP, etc. — scan raw bytes for C2PA/JUMBF signature
		if !hasC2PA {
			rawC2PA := scanForC2PA(f)
			if v, sw, ver, src := extractC2PAInfo(rawC2PA); v != "" || sw != "" {
				hasC2PA = true
				c2paVendor, c2paSw, c2paVer, c2paSource = v, sw, ver, src
			}
			if hasC2PA {
				printC2PAInfo(w, c2paVendor, c2paSw, c2paVer, c2paSource)
			}
		}
	}

	return nil
}

// printC2PAInfo writes the C2PA watermark section.
func printC2PAInfo(w io.Writer, vendor, software, version, source string) {
	fmt.Fprintf(w, "  Watermark: C2PA Content Credentials\n")
	if vendor != "" {
		fmt.Fprintf(w, "    Vendor:   %s\n", vendor)
	}
	if software != "" {
		s := software
		if version != "" {
			s += " " + version
		}
		fmt.Fprintf(w, "    Software: %s\n", s)
	}
	if source != "" {
		fmt.Fprintf(w, "    Source:   %s\n", source)
	}
}

// scanForC2PA does a raw byte scan for JUMBF/C2PA signatures.
// Useful for formats like WebP, GIF, BMP that the library doesn't support.
func scanForC2PA(r io.Reader) []byte {
	data, err := io.ReadAll(r)
	if err != nil || len(data) == 0 {
		return nil
	}
	// JUMBF boxes start with "jumb" (4-byte magic)
	// C2PA uses JUMBF as its container format — if we find it, C2PA is present.
	idx := bytes.Index(data, []byte("jumb"))
	if idx < 0 {
		idx = bytes.Index(data, []byte("C2PA"))
	}
	if idx < 0 {
		// China TC260 AIGC label (GB 45438-2025)
		idx = bytes.Index(data, []byte("TC260:AIGC"))
	}
	if idx < 0 {
		// CNIPA / Chinese digital watermark standards
		idx = bytes.Index(data, []byte("<TC260"))
	}
	if idx < 0 {
		return nil
	}
	// Return surrounding bytes for string extraction
	start := idx
	if start > 64 {
		start -= 64
	}
	end := idx + 4096
	if end > len(data) {
		end = len(data)
	}
	return data[start:end]
}

// truncateMeta shortens long metadata values for display.
func truncateMeta(s string) string {
	// Collapse whitespace
	fields := strings.Fields(s)
	s = strings.Join(fields, " ")
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

// humanSize formats a byte count as a human-readable string.
func humanSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024*1024:
		return fmt.Sprintf("%.2f GB", float64(bytes)/(1024*1024*1024))
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.2f MB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.2f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// ── PNG chunk parsing ──

// readPNGChunks reads PNG chunks and returns:
//   - textChunks: tEXt/zTXt/iTXt keyword→value pairs
//   - c2paData: raw bytes of the first C2PA-related custom chunk (caBX, C2PA, etc.)
//
// The reader must be at offset 0.
func readPNGChunks(r io.Reader) (textChunks map[string]string, c2paData []byte) {
	textChunks = make(map[string]string)

	sig := make([]byte, 8)
	if _, err := io.ReadFull(r, sig); err != nil {
		return nil, nil
	}

	br := bufio.NewReader(r)
	for {
		hdr := make([]byte, 8)
		if _, err := io.ReadFull(br, hdr); err != nil {
			break
		}
		length := binary.BigEndian.Uint32(hdr[:4])
		chunkType := string(hdr[4:8])

		data := make([]byte, length)
		if _, err := io.ReadFull(br, data); err != nil {
			break
		}

		crc := make([]byte, 4)
		if _, err := io.ReadFull(br, crc); err != nil {
			break
		}

		switch chunkType {
		case "tEXt":
			if k, v, ok := parsePNGText(data); ok {
				textChunks[k] = v
			}
		case "zTXt":
			if k, v, ok := parsePNGCompressedText(data); ok {
				textChunks[k] = v
			}
		case "iTXt":
			if k, v, ok := parsePNGInternationalText(data); ok {
				textChunks[k] = v
			}
		case "C2PA", "caBX":
			if c2paData == nil {
				c2paData = data
			}
		case "AIGC":
			// China TC260 AIGC label (GB 45438-2025)
			textChunks["TC260"] = string(data)
		case "IEND":
			break
		}
	}

	return textChunks, c2paData
}

// extractC2PAInfo scans JUMBF/C2PA binary data for human-readable
// provenance strings (vendor, software agent, source type).
func extractC2PAInfo(data []byte) (vendor, software, version, sourceType string) {
	if len(data) == 0 {
		return "", "", "", ""
	}

	// Scan for printable ASCII runs
	var current []byte
	strSet := make(map[string]bool)
	addString := func(s string) {
		if len(s) > 4 && !strSet[s] {
			strSet[s] = true
		}
	}

	for _, b := range data {
		if b >= 32 && b < 127 {
			current = append(current, b)
		} else {
			if len(current) > 4 {
				addString(string(current))
			}
			current = nil
		}
	}
	if len(current) > 4 {
		addString(string(current))
	}

	// Extract vendor from organization names
	for s := range strSet {
		low := strings.ToLower(s)
		if strings.Contains(low, "openai") && vendor == "" {
			vendor = s
		}
	}

	// Extract software name + version from entries like "gpt-image" + "2.0"
	for s := range strSet {
		low := strings.ToLower(s)
		if strings.Contains(low, "gpt") || strings.Contains(low, "dall") ||
			strings.Contains(low, "midjourney") || strings.Contains(low, "stable") ||
			strings.Contains(low, "firefly") || strings.Contains(low, "imagen") {
			software = s
		}
		if len(s) <= 10 && len(s) > 0 && (s[0] >= '0' && s[0] <= '9') &&
			strings.Contains(s, ".") && version == "" {
			// Looks like a version number like "2.0"
			version = s
		}
	}

	// Extract digitalSourceType (last path segment of URL)
	for s := range strSet {
		if strings.Contains(s, "digitalsourcetype/") {
			if idx := strings.LastIndex(s, "/"); idx >= 0 && idx+1 < len(s) {
				sourceType = s[idx+1:]
				// Convert camelCase to readable
				sourceType = camelToWords(sourceType)
			}
		}
	}

	return vendor, software, version, sourceType
}

// tc260ProviderCodes maps ContentProducer entity codes (chars 2-21 of the
// 27-char full code) to provider names. To discover a new code:
//
//	apimart-cli preview --verbose <ai-generated-image.png>
//
// then copy the ContentProducer value and share it to add here.
// References: GB 45438-2025, TC260 service provider encoding rules.
var tc260ProviderCodes = map[string]string{
	"1191110102MACQD9K640": "字节跳动 (ByteDance) — 豆包 / 即梦 / 火山引擎",
	"1191110108MA01KP2T5U": "智谱AI (Zhipu) — 清言 / GLM",
	"1191110000802100433B": "百度 (Baidu) — 文心一言",
	"119144030071526726XG": "腾讯 (Tencent) — 混元",
	"1191330106MA2CFLDG4R": "阿里巴巴 (Alibaba) — 通义千问 / 通义万相",
	// ── 以下为常见大模型厂商（需要实际图片确认编码） ──
	// To add: generate an image with the provider's tool, run
	//   apimart-cli preview --verbose <image>
	// and submit the ContentProducer code as a PR.
	// "????????????????????": "DeepSeek — 深度求索",
	// "????????????????????": "百度 (Baidu) — 文心一言",
	// "????????????????????": "阿里巴巴 (Alibaba) — 通义千问 / 通义万相",
	// "????????????????????": "腾讯 (Tencent) — 混元",
	// "????????????????????": "小米 (Xiaomi) — MiMo",
	// "????????????????????": "美团 (Meituan) — 美团AI",
	// "????????????????????": "科大讯飞 (iFlytek) — 星火",
	// "????????????????????": "月之暗面 (Moonshot) — Kimi",
	// "????????????????????": "百川智能 (Baichuan) — 百川",
	// "????????????????????": "MiniMax — 海螺AI",
	// "????????????????????": "商汤科技 (SenseTime) — 日日新",
	// "????????????????????": "昆仑万维 (Kunlun) — 天工AI",
	// "????????????????????": "华为 (Huawei) — 盘古",
}

// resolveContentProducer looks up the entity code portion of a TC260
// ContentProducer code and returns a recognizable provider name.
func resolveContentProducer(code string) string {
	// Full code is 27 chars: version(2) + entity(20) + service(5)
	// Try matching from longest prefix
	if len(code) >= 22 {
		entityCode := code[2:22] // extract the entity code (20 chars)
		if name, ok := tc260ProviderCodes[entityCode]; ok {
			return name
		}
	}
	// Fallback: try exact match
	if name, ok := tc260ProviderCodes[code]; ok {
		return name
	}
	return ""
}

// camelToWords converts "trainedAlgorithmicMedia" to "Trained Algorithmic Media".
func camelToWords(s string) string {
	if s == "" {
		return ""
	}
	var words []string
	var cur []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			if len(cur) > 0 {
				words = append(words, string(cur))
			}
			cur = []rune{r}
		} else {
			cur = append(cur, r)
		}
	}
	if len(cur) > 0 {
		words = append(words, string(cur))
	}
	return strings.Join(words, " ")
}

// parsePNGText parses a tEXt chunk: null-terminated keyword + Latin-1 text.
func parsePNGText(data []byte) (key, value string, ok bool) {
	nullIdx := bytes.IndexByte(data, 0)
	if nullIdx < 0 || nullIdx > 79 {
		return "", "", false
	}
	key = string(data[:nullIdx])
	value = string(data[nullIdx+1:])
	return key, value, true
}

// parsePNGCompressedText parses a zTXt chunk:
// null-terminated keyword + compression method (1 byte) + compressed data.
func parsePNGCompressedText(data []byte) (key, value string, ok bool) {
	nullIdx := bytes.IndexByte(data, 0)
	if nullIdx < 0 || nullIdx > 79 || nullIdx+1 >= len(data) {
		return "", "", false
	}
	key = string(data[:nullIdx])
	compressed := data[nullIdx+2:] // skip null + compression method
	if len(compressed) == 0 {
		return key, "", true
	}
	rc, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return "", "", false
	}
	defer rc.Close()
	decompressed, err := io.ReadAll(rc)
	if err != nil {
		return "", "", false
	}
	return key, string(decompressed), true
}

// parsePNGInternationalText parses an iTXt chunk.
func parsePNGInternationalText(data []byte) (key, value string, ok bool) {
	nullIdx := bytes.IndexByte(data, 0)
	if nullIdx < 0 || nullIdx > 79 || nullIdx+3 >= len(data) {
		return "", "", false
	}
	key = string(data[:nullIdx])
	rest := data[nullIdx+1:] // rest = [comp_flag, comp_method, lang\0, trans_keyword\0, text...]
	if len(rest) < 3 {
		return "", "", false
	}
	compressionFlag := rest[0]
	// compressionMethod := rest[1] — always 0 for deflate

	// Language tag starts at rest[2], null-terminated
	langStart := 2
	langEnd := langStart
	for langEnd < len(rest) && rest[langEnd] != 0 {
		langEnd++
	}
	if langEnd >= len(rest) {
		return "", "", false
	}
	// Translated keyword: null-terminated after language tag
	tkStart := langEnd + 1
	tkEnd := tkStart
	for tkEnd < len(rest) && rest[tkEnd] != 0 {
		tkEnd++
	}
	if tkEnd >= len(rest) {
		return "", "", false
	}
	textBytes := rest[tkEnd+1:]

	if compressionFlag == 1 {
		rc, err := zlib.NewReader(bytes.NewReader(textBytes))
		if err != nil {
			return "", "", false
		}
		defer rc.Close()
		decompressed, err := io.ReadAll(rc)
		if err != nil {
			return "", "", false
		}
		value = string(decompressed)
	} else {
		value = string(textBytes)
	}
	return key, value, true
}

// ── JPEG metadata parsing ──

type jpegInfo struct {
	comment   string
	software  string
	hasC2PA   bool
	tc260Data string // China TC260 AIGC label (from EXIF/XMP)
}

// readJPEGInfo reads JPEG markers and extracts comment and software tags.
func readJPEGInfo(r io.Reader) jpegInfo {
	var info jpegInfo

	// JPEG starts with FF D8 (SOI)
	br := bufio.NewReader(r)
	soi := make([]byte, 2)
	if _, err := io.ReadFull(br, soi); err != nil || soi[0] != 0xFF || soi[1] != 0xD8 {
		return info
	}

	for {
		// Read marker
		b, err := br.ReadByte()
		if err != nil {
			break
		}
		if b != 0xFF {
			break
		}
		// Skip 0xFF padding bytes
		for b == 0xFF {
			b, err = br.ReadByte()
			if err != nil {
				return info
			}
		}
		markerType := b

		// SOS — start of scan data, stop parsing
		if markerType == 0xDA {
			break
		}
		// EOI / RST markers — no segment length
		if markerType == 0xD9 || (markerType >= 0xD0 && markerType <= 0xD7) {
			continue
		}

		// Read segment length (2 bytes, big-endian, includes self)
		lenBytes := make([]byte, 2)
		if _, err := io.ReadFull(br, lenBytes); err != nil {
			break
		}
		segLen := int(binary.BigEndian.Uint16(lenBytes)) - 2
		if segLen <= 0 {
			continue
		}

		segData := make([]byte, segLen)
		if _, err := io.ReadFull(br, segData); err != nil {
			break
		}

		switch markerType {
		case 0xFE: // COM — comment
			info.comment = strings.TrimSpace(string(segData))
		case 0xE1: // APP1 — EXIF / XMP
			if len(segData) > 6 && string(segData[:6]) == "Exif\000\000" {
				if sw := readExifSoftware(segData[6:]); sw != "" {
					info.software = sw
				}
				// Scan EXIF for TC260 AIGC label (stored as JSON in EXIF)
				if tc260 := scanEXIFForTC260(segData[6:]); tc260 != "" {
					info.tc260Data = tc260
				}
			}
		case 0xEB: // APP11 — JUMBF / C2PA
			if len(segData) > 4 && string(segData[:4]) == "JUMBF" {
				info.hasC2PA = true
			}
		}
	}

	return info
}

// readExifSoftware does a minimal parse of the EXIF IFD0 to find the Software tag (0x0131).
// exifData starts after the "Exif\0\0" header — the TIFF header.
func readExifSoftware(exifData []byte) string {
	if len(exifData) < 8 {
		return ""
	}
	// TIFF header: byte order (2 bytes) + magic (2 bytes) + offset to IFD0 (4 bytes)
	byteOrder := string(exifData[:2])
	var order binary.ByteOrder
	switch byteOrder {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return ""
	}

	if order.Uint16(exifData[2:4]) != 42 {
		return ""
	}

	ifdOffset := int(order.Uint32(exifData[4:8]))
	if ifdOffset+2 > len(exifData) {
		return ""
	}

	numEntries := int(order.Uint16(exifData[ifdOffset : ifdOffset+2]))
	offset := ifdOffset + 2
	for i := 0; i < numEntries && offset+12 <= len(exifData); i++ {
		tag := order.Uint16(exifData[offset : offset+2])
		// typeField := order.Uint16(exifData[offset+2 : offset+4])
		// count := order.Uint32(exifData[offset+4 : offset+8])
		valueOffset := order.Uint32(exifData[offset+8 : offset+12])

		if tag == 0x0131 { // Software
			// Try direct as ASCII (short values stored inline)
			sw := strings.TrimRight(string(exifData[offset+8:offset+12]), "\000")
			if !hasControlChars(sw) {
				return sw
			}
			// Otherwise read at value offset if reasonable
			if int(valueOffset)+64 <= len(exifData) {
				end := int(valueOffset) + 64
				if end > len(exifData) {
					end = len(exifData)
				}
				sw = strings.TrimRight(string(exifData[valueOffset:end]), "\000")
				if !hasControlChars(sw) && sw != "" {
					return sw
				}
			}
		}
		offset += 12
	}
	return ""
}

// scanEXIFForTC260 scans EXIF TIFF data for a TC260 AIGC label.
// The label is stored as a JSON object within the EXIF data, typically
// as {"AIGC": {"Label":"1","ContentProducer":"...",...}}.
// This is a raw byte scan since the label can be at any IFD/offset.
func scanEXIFForTC260(exifData []byte) string {
	// Look for the JSON key "AIGC" followed by the TC260 fields
	idx := bytes.Index(exifData, []byte(`"AIGC"`))
	if idx < 0 {
		// Also try "TC260" as key
		idx = bytes.Index(exifData, []byte(`"TC260"`))
	}
	if idx < 0 {
		return ""
	}
	// Find the value part after the key: look for { after :
	afterKey := exifData[idx:]
	braceIdx := bytes.IndexByte(afterKey, '{')
	if braceIdx < 0 {
		return ""
	}
	// Find matching closing brace (simple brace counting)
	depth := 0
	end := braceIdx
	for end < len(afterKey) {
		switch afterKey[end] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				jsonStr := string(afterKey[braceIdx : end+1])
				// Extract the ContentProducer from the inner JSON
				var parsed struct {
					AIGC struct {
						ContentProducer string `json:"ContentProducer"`
					} `json:"AIGC"`
				}
				if json.Unmarshal([]byte(jsonStr), &parsed) == nil && parsed.AIGC.ContentProducer != "" {
					return jsonStr
				}
				// Try flat parsing
				var flat map[string]string
				if json.Unmarshal([]byte(jsonStr), &flat) == nil {
					if cp, ok := flat["ContentProducer"]; ok && cp != "" {
						return jsonStr
					}
				}
				// Try AIGC as nested map
				var nested map[string]map[string]string
				if json.Unmarshal([]byte(jsonStr), &nested) == nil {
					if aigc, ok := nested["AIGC"]; ok {
						if cp, ok := aigc["ContentProducer"]; ok && cp != "" {
							return jsonStr
						}
					}
				}
				return ""
			}
		}
		end++
	}
	return ""
}

// printTC260 formats and prints the TC260 AIGC label data.
func printTC260(w io.Writer, tc260data string) {
	decoded := strings.TrimSpace(html.UnescapeString(tc260data))
	var pretty string

	// Some providers wrap the JSON in a JSON string: "{\"Label\":...}"
	// Detect this and unwrap one level.
	if strings.HasPrefix(decoded, `"`) && strings.HasSuffix(decoded, `"`) {
		var inner string
		if json.Unmarshal([]byte(decoded), &inner) == nil {
			decoded = inner
		}
	}

	var parsed map[string]string
	if json.Unmarshal([]byte(decoded), &parsed) == nil {
		pretty = formatTC260(parsed)
	} else {
		// Try {"AIGC": {...}} nested format
		var nested map[string]map[string]string
		if json.Unmarshal([]byte(decoded), &nested) == nil {
			if aigc, ok := nested["AIGC"]; ok {
				pretty = formatTC260(aigc)
			}
		}
	}

	fmt.Fprintf(w, "  Watermark: TC260 AIGC Label (China GB 45438-2025)\n")
	if pretty != "" {
		fmt.Fprintln(w, pretty)
	} else {
		fmt.Fprintf(w, "    Data: %s\n", truncateMeta(tc260data))
	}
}

// formatTC260 formats a parsed TC260 label map into display lines.
func formatTC260(data map[string]string) string {
	var result strings.Builder
	for k, v := range data {
		if v == "" {
			continue
		}
		if k == "ContentProducer" {
			if name := resolveContentProducer(v); name != "" {
				result.WriteString(fmt.Sprintf("    Provider: %s\n", name))
				continue
			}
		}
		result.WriteString(fmt.Sprintf("    %s: %s\n", k, v))
	}
	return strings.TrimSuffix(result.String(), "\n")
}

// hasControlChars returns true if the string contains control characters (except null).
func hasControlChars(s string) bool {
	for _, r := range s {
		if r > 0 && r < 32 {
			return true
		}
	}
	return false
}

// isImageFile returns true if the file extension is a supported image type.
func isImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}

// mimeFromExt returns the MIME type for a given image extension.
func mimeFromExt(ext string) string {
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	}
	return ""
}
