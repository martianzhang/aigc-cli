package service

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"html"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/richardwooding/c2pa"
)

// TC260 field keys.
const ContentProducerKey = "ContentProducer"

// Known watermark provider names (map TC260 ContentProducer to config names).
const (
	ProviderGemini = "gemini"
	ProviderDoubao = "doubao"
	ProviderJimeng = "jimeng"
)

// DetectResult holds structured image detection data.
type DetectResult struct {
	Path      string            `json:"path"`
	Size      int64             `json:"size"`
	SizeHuman string            `json:"size_human"`
	Modified  time.Time         `json:"modified"`
	Format    string            `json:"format"`
	Width     int               `json:"width"`
	Height    int               `json:"height"`
	C2PA      *C2PAResult       `json:"c2pa,omitempty"`
	TC260     *TC260Result      `json:"tc260,omitempty"`
	SynthID   *SynthIDResult    `json:"synthid,omitempty"`
	AIDetect  *AIDetectResult   `json:"ai_detect,omitempty"`
	Camera    *CameraInfo       `json:"camera,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Comment   string            `json:"comment,omitempty"`
	Software  string            `json:"software,omitempty"`
}

// CameraInfo holds EXIF camera metadata extracted from JPEG images.
type CameraInfo struct {
	Make         string `json:"make,omitempty"`
	Model        string `json:"model,omitempty"`
	LensModel    string `json:"lens_model,omitempty"`
	FocalLength  string `json:"focal_length,omitempty"`
	FNumber      string `json:"f_number,omitempty"`
	ISO          string `json:"iso,omitempty"`
	ExposureTime string `json:"exposure_time,omitempty"`
}

// C2PAResult holds C2PA watermark detection results.
type C2PAResult struct {
	Present  bool   `json:"present"`
	Vendor   string `json:"vendor,omitempty"`
	Software string `json:"software,omitempty"`
	Version  string `json:"version,omitempty"`
	Source   string `json:"source,omitempty"`
}

// TC260Result holds TC260 AIGC label detection results.
type TC260Result struct {
	Present  bool              `json:"present"`
	Data     string            `json:"data,omitempty"`
	Provider string            `json:"provider,omitempty"`
	Fields   map[string]string `json:"fields,omitempty"`
}

// SynthIDResult holds SynthID watermark inference results.
// Note: this is currently metadata-based inference from C2PA manifests.
// Pixel-level detection requires additional spectral analysis.
type SynthIDResult struct {
	Present   bool   `json:"present"`
	Likely    bool   `json:"likely"`
	Source    string `json:"source,omitempty"`
	Inference string `json:"inference,omitempty"`
}

// AIDetectResult holds the multi-signal fusion AIGC detection result.
type AIDetectResult struct {
	AIGenRate float64 `json:"ai_gen_rate"`       // 0-1, higher = more likely AI
	Emoji     string  `json:"emoji"`             // summary emoji (🟢🟡🟠🔴🤖)
	Summary   string  `json:"summary"`           // human-readable summary
	Details   string  `json:"details,omitempty"` // signal breakdown
}

// DetectImage analyzes an image file and returns structured detection data
// including file stats, format, dimensions, C2PA info, TC260 info, and metadata.
func DetectImage(path string) (*DetectResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	result := &DetectResult{
		Path:      path,
		Size:      info.Size(),
		SizeHuman: humanSize(info.Size()),
		Modified:  info.ModTime(),
		Metadata:  make(map[string]string),
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	config, format, err := image.DecodeConfig(f)
	if err != nil {
		result.Format = "unknown"
		return result, nil
	}
	result.Format = strings.ToUpper(format)
	result.Width = config.Width
	result.Height = config.Height

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

		// PNG text chunks (skip internal XMP metadata)
		delete(textChunks, "XML:com.adobe.xmp")
		for k, v := range textChunks {
			result.Metadata[k] = v
		}

	case "jpeg":
		jf := readJPEGInfo(f)
		if !hasC2PA && jf.hasC2PA {
			hasC2PA = true
			c2paSource = "C2PA data detected (library parse failed)"
		}
		result.Comment = jf.comment
		result.Software = jf.software
		result.Camera = jf.camera
		// TC260 AIGC label in EXIF (JPEG stores it in APP1 JSON)
		if !hasTC260 && jf.tc260Data != "" {
			tc260data = jf.tc260Data
			hasTC260 = true
		}

	case "gif":
		if comment := readGIFInfo(f); comment != "" {
			result.Comment = comment
		}
		// Raw byte scan for C2PA/JUMBF (no library support for GIF)
		if !hasC2PA {
			f.Seek(0, io.SeekStart)
			rawC2PA := scanForC2PA(f)
			if v, sw, ver, src := extractC2PAInfo(rawC2PA); v != "" || sw != "" {
				hasC2PA = true
				c2paVendor, c2paSw, c2paVer, c2paSource = v, sw, ver, src
			}
		}

	case "webp":
		f.Seek(0, io.SeekStart)
		webpData := readWebPInfo(f)
		if webpData.comment != "" {
			result.Comment = webpData.comment
		}
		if !hasC2PA && webpData.hasC2PA {
			hasC2PA = true
			c2paSource = "C2PA data detected in WebP RIFF container"
		}
		// Fallback: raw byte scan for JUMBF/C2PA signatures
		if !hasC2PA {
			f.Seek(0, io.SeekStart)
			rawC2PA := scanForC2PA(f)
			if v, sw, ver, src := extractC2PAInfo(rawC2PA); v != "" || sw != "" {
				hasC2PA = true
				c2paVendor, c2paSw, c2paVer, c2paSource = v, sw, ver, src
			}
		}

	default:
		// BMP and other formats — scan raw bytes for C2PA/JUMBF signature
		if !hasC2PA {
			rawC2PA := scanForC2PA(f)
			if v, sw, ver, src := extractC2PAInfo(rawC2PA); v != "" || sw != "" {
				hasC2PA = true
				c2paVendor, c2paSw, c2paVer, c2paSource = v, sw, ver, src
			}
		}
	}

	if hasC2PA {
		result.C2PA = &C2PAResult{
			Present:  true,
			Vendor:   c2paVendor,
			Software: c2paSw,
			Version:  c2paVer,
			Source:   c2paSource,
		}
		// Infer SynthID from C2PA metadata
		result.SynthID = inferSynthID(c2paVendor, c2paSw, c2paSource)
	}

	if hasTC260 {
		result.TC260 = parseTC260Result(tc260data)
	}

	return result, nil
}

// inferSynthID returns a SynthIDResult based on C2PA metadata heuristics.
// Google pairs SynthID with C2PA for Imagen/Gemini-generated images.
// OpenAI also uses invisible watermarks (possibly SynthID) for DALL-E images.
func inferSynthID(vendor, software, source string) *SynthIDResult {
	lowVendor := strings.ToLower(vendor)
	lowSoftware := strings.ToLower(software)
	lowSource := strings.ToLower(source)

	// Google products
	if strings.Contains(lowVendor, "google") ||
		strings.Contains(lowSoftware, "imagen") ||
		strings.Contains(lowSoftware, "gemini") ||
		strings.Contains(lowSoftware, "goo_") {
		return &SynthIDResult{
			Present:   true,
			Likely:    true,
			Source:    vendor,
			Inference: "C2PA manifest from Google — SynthID watermark likely embedded",
		}
	}

	// OpenAI products (DALL-E, GPT-Image)
	if strings.Contains(lowVendor, "openai") ||
		strings.Contains(lowSoftware, "dall") ||
		strings.Contains(lowSoftware, "gpt-image") {
		return &SynthIDResult{
			Present:   true,
			Likely:    true,
			Source:    vendor,
			Inference: "C2PA manifest from OpenAI — invisible watermark likely embedded (SynthID or similar)",
		}
	}

	// Generic AI-generated flag
	if strings.Contains(lowSource, "ai generated") || strings.Contains(lowSource, "algorithmic") {
		return &SynthIDResult{
			Present:   false,
			Likely:    true,
			Source:    source,
			Inference: "AI-generated content flagged in C2PA — invisible watermark may be present",
		}
	}

	return nil
}

// parseTC260Result parses TC260 data into a structured result.
func parseTC260Result(tc260data string) *TC260Result {
	result := &TC260Result{
		Present: true,
		Data:    tc260data,
		Fields:  make(map[string]string),
	}

	decoded := strings.TrimSpace(html.UnescapeString(tc260data))

	// Some providers wrap the JSON in a JSON string: "{\"Label":...}"
	if strings.HasPrefix(decoded, `"`) && strings.HasSuffix(decoded, `"`) {
		var inner string
		if json.Unmarshal([]byte(decoded), &inner) == nil {
			decoded = inner
		}
	}

	var parsed map[string]string
	if json.Unmarshal([]byte(decoded), &parsed) == nil {
		for k, v := range parsed {
			result.Fields[k] = v
		}
	} else {
		var nested map[string]map[string]string
		if json.Unmarshal([]byte(decoded), &nested) == nil {
			if aigc, ok := nested["AIGC"]; ok {
				for k, v := range aigc {
					result.Fields[k] = v
				}
			}
		}
	}

	if cp, ok := result.Fields[ContentProducerKey]; ok {
		result.Provider = resolveContentProducer(cp)
	}

	return result
}

// PrintImageInfo writes image metadata to w.
// Backward-compatible wrapper that prints everything (verbose mode).
func PrintImageInfo(w io.Writer, path string) error {
	result, err := DetectImage(path)
	if err != nil {
		return err
	}
	return PrintDetectResult(w, result, true)
}

// PrintDetectResult writes detection results to w.
// When verbose=false: only print C2PA and TC260 watermark info.
// When verbose=true: print everything (file stats, dimensions, format, all metadata).
func PrintDetectResult(w io.Writer, result *DetectResult, verbose bool) error {
	if verbose {
		fmt.Fprintf(w, "━━━ %s ━━━\n", filepath.Base(result.Path))
		fmt.Fprintf(w, "  Size:     %s\n", result.SizeHuman)
		fmt.Fprintf(w, "  Modified: %s\n", result.Modified.Format("2006-01-02 15:04:05"))
		if result.Format != "unknown" {
			fmt.Fprintf(w, "  Format:   %s\n", result.Format)
			fmt.Fprintf(w, "  Dims:     %d x %d\n", result.Width, result.Height)
		} else {
			fmt.Fprintf(w, "  (not a recognized image format)\n")
		}
	}

	if result.C2PA != nil && result.C2PA.Present {
		printC2PAInfo(w, result.C2PA.Vendor, result.C2PA.Software, result.C2PA.Version, result.C2PA.Source)
	}

	if result.SynthID != nil {
		printSynthID(w, result.SynthID)
	}

	if result.TC260 != nil && result.TC260.Present {
		printTC260(w, result.TC260)
	}

	if result.AIDetect != nil {
		printAIDetect(w, result.AIDetect)
	}

	if verbose {
		printCameraInfo(w, result.Camera)
		if result.Comment != "" {
			fmt.Fprintf(w, "  Comment:  %s\n", truncateMeta(result.Comment))
		}
		if result.Software != "" {
			fmt.Fprintf(w, "  Software: %s\n", truncateMeta(result.Software))
		}
		if len(result.Metadata) > 0 {
			fmt.Fprintf(w, "  Metadata:\n")
			for _, key := range []string{"Software", "Description", "Generation", "parameters", "Title"} {
				if v, ok := result.Metadata[key]; ok {
					fmt.Fprintf(w, "    %s: %s\n", key, truncateMeta(v))
					delete(result.Metadata, key)
				}
			}
			for k, v := range result.Metadata {
				fmt.Fprintf(w, "    %s: %s\n", k, truncateMeta(v))
			}
		}
	}

	return nil
}

// PrintDetectResultJSON writes detection results as JSON to w.
func PrintDetectResultJSON(w io.Writer, result *DetectResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
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

// printSynthID formats and prints SynthID watermark inference.
func printSynthID(w io.Writer, result *SynthIDResult) {
	if result.Present {
		fmt.Fprintf(w, "  Watermark: SynthID (Google invisible watermark)\n")
	} else if result.Likely {
		fmt.Fprintf(w, "  Watermark: SynthID likely (inferred from metadata)\n")
	} else {
		return
	}
	if result.Source != "" {
		fmt.Fprintf(w, "    Source:   %s\n", result.Source)
	}
	if result.Inference != "" {
		fmt.Fprintf(w, "    Note:     %s\n", result.Inference)
	}
}

// printAIDetect prints the fused AIGC detection result with emoji.
func printAIDetect(w io.Writer, result *AIDetectResult) {
	fmt.Fprintf(w, "  AI Detect:  %s\n", result.Summary)
	if result.Details != "" {
		fmt.Fprintf(w, "    %s\n", result.Details)
	}
}

// printTC260 formats and prints the TC260 AIGC label data.
func printTC260(w io.Writer, result *TC260Result) {
	fmt.Fprintf(w, "  Watermark: TC260 AIGC Label (China GB 45438-2025)\n")
	if result.Provider != "" {
		fmt.Fprintf(w, "    Provider: %s\n", result.Provider)
	}
	pretty := formatTC260(result.Fields)
	if pretty != "" {
		fmt.Fprintln(w, pretty)
	} else if result.Data != "" {
		fmt.Fprintf(w, "    Data: %s\n", truncateMeta(result.Data))
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
chunks:
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
			break chunks
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
//	aigc-cli detect --verbose <ai-generated-image.png>
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
	//   aigc-cli detect --verbose <image>
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

// ── GIF comment extraction ──

// readGIFInfo reads GIF comment extension blocks (0xFE).
func readGIFInfo(r io.Reader) string {
	br := bufio.NewReader(r)

	// Skip header (6 bytes: GIF87a/GIF89a) + logical screen descriptor (7 bytes)
	if _, err := br.Discard(13); err != nil {
		return ""
	}

	// Scan blocks for comment extensions
	for {
		b, err := br.ReadByte()
		if err != nil {
			break
		}
		switch b {
		case 0x21: // Extension introducer
			label, err := br.ReadByte()
			if err != nil {
				return ""
			}
			if label == 0xFE { // Comment extension
				if comment := readGIFSubBlocks(br); comment != "" {
					return comment
				}
			} else {
				skipGIFBlocks(br)
			}
		case 0x2C: // Image descriptor
			// Skip image descriptor + local color table + LZW data
			skipGIFImageData(br)
		case 0x3B: // Trailer
			return ""
		default:
			return ""
		}
	}
	return ""
}

// readGIFSubBlocks reads GIF sub-blocks until a zero-length terminator.
func readGIFSubBlocks(br *bufio.Reader) string {
	var result []byte
	for {
		size, err := br.ReadByte()
		if err != nil || size == 0 {
			break
		}
		block := make([]byte, size)
		if _, err := io.ReadFull(br, block); err != nil {
			return ""
		}
		result = append(result, block...)
	}
	return strings.TrimSpace(string(result))
}

// skipGIFBlocks skips sub-blocks until a zero-length terminator.
func skipGIFBlocks(br *bufio.Reader) {
	for {
		size, err := br.ReadByte()
		if err != nil || size == 0 {
			break
		}
		if _, err := br.Discard(int(size)); err != nil {
			return
		}
	}
}

// skipGIFImageData skips a GIF image descriptor + local color table + LZW data.
func skipGIFImageData(br *bufio.Reader) {
	// Image descriptor: 9 bytes
	if _, err := br.Discard(9); err != nil {
		return
	}
	// Skip LZW minimum code size (1 byte) + sub-blocks
	skipGIFBlocks(br)
}

// ── WebP RIFF chunk parsing ──

// webpInfo holds metadata extracted from WebP RIFF container.
type webpInfo struct {
	comment string
	hasC2PA bool
}

// readWebPInfo reads WebP RIFF container for EXIF/XMP/C2PA metadata.
func readWebPInfo(r io.Reader) webpInfo {
	var info webpInfo

	data, err := io.ReadAll(r)
	if err != nil || len(data) < 12 {
		return info
	}

	// Validate RIFF header
	if string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return info
	}

	fileSize := int(binary.LittleEndian.Uint32(data[4:8]))
	if fileSize+8 > len(data) {
		fileSize = len(data) - 8
	}

	pos := 12
	for pos+8 <= fileSize+8 && pos+8 <= len(data) {
		chunkID := string(data[pos : pos+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))

		if chunkSize < 0 || pos+8+chunkSize > len(data) {
			break
		}

		switch chunkID {
		case "EXIF":
			exifStart := pos + 8
			if chunkSize > 6 && string(data[exifStart:exifStart+6]) == "Exif\000\000" {
				exifData := data[exifStart+6 : exifStart+chunkSize]
				if sw := readExifSoftware(exifData); sw != "" {
					info.comment = "EXIF: " + sw
				}
			}
			// Check for JUMBF/C2PA in EXIF chunk
			if bytes.Contains(data[pos:pos+8+chunkSize], []byte("jumb")) ||
				bytes.Contains(data[pos:pos+8+chunkSize], []byte("C2PA")) {
				info.hasC2PA = true
			}
		case "XMP":
			if !info.hasC2PA {
				if bytes.Contains(data[pos:pos+8+chunkSize], []byte("jumb")) {
					info.hasC2PA = true
				}
			}
		}

		pos += 8 + chunkSize
		// Chunk data is padded to even bytes
		if chunkSize%2 != 0 {
			pos++
		}
	}

	return info
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
	tc260Data string      // China TC260 AIGC label (from EXIF/XMP)
	camera    *CameraInfo // EXIF camera metadata
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
				exifData := segData[6:]
				if sw := readExifSoftware(exifData); sw != "" {
					info.software = sw
				}
				// Scan EXIF for TC260 AIGC label (stored as JSON in EXIF)
				if tc260 := scanEXIFForTC260(exifData); tc260 != "" {
					info.tc260Data = tc260
				}
				info.camera = readExifCamera(exifData)
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
					if cp, ok := flat[ContentProducerKey]; ok && cp != "" {
						return jsonStr
					}
				}
				// Try AIGC as nested map
				var nested map[string]map[string]string
				if json.Unmarshal([]byte(jsonStr), &nested) == nil {
					if aigc, ok := nested["AIGC"]; ok {
						if cp, ok := aigc[ContentProducerKey]; ok && cp != "" {
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

// formatTC260 formats a parsed TC260 label map into display lines.
func formatTC260(data map[string]string) string {
	var result strings.Builder
	for k, v := range data {
		if v == "" {
			continue
		}
		if k == ContentProducerKey {
			if name := resolveContentProducer(v); name != "" {
				fmt.Fprintf(&result, "    Provider: %s\n", name)
				continue
			}
		}
		fmt.Fprintf(&result, "    %s: %s\n", k, v)
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

// printCameraInfo formats and prints EXIF camera metadata.
func printCameraInfo(w io.Writer, camera *CameraInfo) {
	if camera == nil || (camera.Make == "" && camera.Model == "" && camera.LensModel == "" &&
		camera.FocalLength == "" && camera.FNumber == "" && camera.ISO == "" && camera.ExposureTime == "") {
		fmt.Fprintf(w, "  Camera:   (none)\n")
		fmt.Fprintf(w, "    (no camera EXIF found — likely not a real photograph)\n")
		return
	}
	fmt.Fprintf(w, "  Camera:\n")
	if camera.Make != "" {
		fmt.Fprintf(w, "    Make:         %s\n", camera.Make)
	}
	if camera.Model != "" {
		fmt.Fprintf(w, "    Model:        %s\n", camera.Model)
	}
	if camera.LensModel != "" {
		fmt.Fprintf(w, "    Lens:         %s\n", camera.LensModel)
	}
	if camera.FocalLength != "" {
		fmt.Fprintf(w, "    FocalLength:  %s\n", camera.FocalLength)
	}
	if camera.FNumber != "" {
		fmt.Fprintf(w, "    Aperture:     f/%s\n", camera.FNumber)
	}
	if camera.ISO != "" {
		fmt.Fprintf(w, "    ISO:          %s\n", camera.ISO)
	}
	if camera.ExposureTime != "" {
		fmt.Fprintf(w, "    Shutter:      %s\n", camera.ExposureTime)
	}
}

// readExifCamera parses EXIF IFD0 and ExifIFD SubIFD for camera metadata tags.
// Returns nil if no camera information is found.
func readExifCamera(exifData []byte) *CameraInfo {
	if len(exifData) < 8 {
		return nil
	}
	byteOrder := string(exifData[:2])
	var order binary.ByteOrder
	switch byteOrder {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil
	}
	if order.Uint16(exifData[2:4]) != 42 {
		return nil
	}
	ifdOffset := int(order.Uint32(exifData[4:8]))
	if ifdOffset+2 > len(exifData) {
		return nil
	}

	var camera CameraInfo
	var exifIFDOffset int // ExifIFD pointer (tag 0x8769)

	// Pass 1: scan IFD0 for Make/Model/LensModel and ExifIFD pointer
	numEntries := int(order.Uint16(exifData[ifdOffset : ifdOffset+2]))
	offset := ifdOffset + 2
	for i := 0; i < numEntries && offset+12 <= len(exifData); i++ {
		tag := order.Uint16(exifData[offset : offset+2])
		typ := order.Uint16(exifData[offset+2 : offset+4])
		count := order.Uint32(exifData[offset+4 : offset+8])

		switch tag {
		case 0x010F: // Make
			camera.Make = readExifASCII(exifData, order, offset, count)
		case 0x0110: // Model
			camera.Model = readExifASCII(exifData, order, offset, count)
		case 0xA434: // LensModel
			camera.LensModel = readExifASCII(exifData, order, offset, count)
		case 0x8769: // ExifIFD pointer — SubIFD with shooting metadata
			if typ == 4 && count == 1 { // LONG
				exifIFDOffset = int(order.Uint32(exifData[offset+8 : offset+12]))
			}
		}
		offset += 12
	}

	// Pass 2: scan ExifIFD SubIFD for shooting metadata (ISO, FNumber, etc.)
	if exifIFDOffset > 0 && exifIFDOffset+2 <= len(exifData) {
		subNumEntries := int(order.Uint16(exifData[exifIFDOffset : exifIFDOffset+2]))
		subOffset := exifIFDOffset + 2
		for i := 0; i < subNumEntries && subOffset+12 <= len(exifData); i++ {
			tag := order.Uint16(exifData[subOffset : subOffset+2])
			typ := order.Uint16(exifData[subOffset+2 : subOffset+4])
			count := order.Uint32(exifData[subOffset+4 : subOffset+8])

			switch tag {
			case 0x920A: // FocalLength
				if camera.FocalLength == "" {
					if num, den := readExifRATIONAL(exifData, order, subOffset, typ, count); den != 0 {
						camera.FocalLength = fmt.Sprintf("%.1fmm", float64(num)/float64(den))
					}
				}
			case 0x829D: // FNumber
				if camera.FNumber == "" {
					if num, den := readExifRATIONAL(exifData, order, subOffset, typ, count); den != 0 {
						camera.FNumber = fmt.Sprintf("%.1f", float64(num)/float64(den))
					}
				}
			case 0x8827: // ISOSpeedRatings
				if camera.ISO == "" {
					if v := readExifSHORT(exifData, order, subOffset, typ, count); v != 0 {
						camera.ISO = fmt.Sprintf("%d", v)
					}
				}
			case 0x829A: // ExposureTime
				if camera.ExposureTime == "" {
					if num, den := readExifRATIONAL(exifData, order, subOffset, typ, count); den != 0 {
						camera.ExposureTime = formatExposureTime(num, den)
					}
				}
			case 0xA434: // LensModel (some cameras put it in SubIFD)
				if camera.LensModel == "" {
					camera.LensModel = readExifASCII(exifData, order, subOffset, count)
				}
			}
			subOffset += 12
		}
	}

	if camera.Make == "" && camera.Model == "" && camera.LensModel == "" &&
		camera.FocalLength == "" && camera.FNumber == "" && camera.ISO == "" && camera.ExposureTime == "" {
		return nil
	}
	return &camera
}

// readExifASCII reads an ASCII string from an EXIF IFD entry.
// The entry starts at entryOffset and the value/offset field is at entryOffset+8.
func readExifASCII(exifData []byte, order binary.ByteOrder, entryOffset int, count uint32) string {
	if count == 0 {
		return ""
	}
	totalSize := int(count) // type=2, size=1 byte per count
	var data []byte
	if totalSize <= 4 {
		// Inline: stored in bytes 8-11 of the 12-byte entry
		data = exifData[entryOffset+8 : entryOffset+8+totalSize]
	} else {
		off := int(order.Uint32(exifData[entryOffset+8 : entryOffset+12]))
		if off+totalSize > len(exifData) {
			return ""
		}
		data = exifData[off : off+totalSize]
	}
	s := strings.TrimRight(string(data), "\x00")
	return strings.TrimSpace(s)
}

// readExifSHORT reads a SHORT value from an EXIF IFD entry.
func readExifSHORT(exifData []byte, order binary.ByteOrder, entryOffset int, typ uint16, count uint32) uint16 {
	if typ != 3 || count != 1 { // SHORT=3
		return 0
	}
	// Inline: 2 bytes stored in the low bytes of the value field
	return order.Uint16(exifData[entryOffset+8 : entryOffset+10])
}

// readExifRATIONAL reads a RATIONAL (two LONGs) from an EXIF IFD entry.
func readExifRATIONAL(exifData []byte, order binary.ByteOrder, entryOffset int, typ uint16, count uint32) (num, den uint32) {
	if typ != 5 || count != 1 { // RATIONAL=5
		return 0, 0
	}
	off := int(order.Uint32(exifData[entryOffset+8 : entryOffset+12]))
	if off+8 > len(exifData) {
		return 0, 0
	}
	num = order.Uint32(exifData[off : off+4])
	den = order.Uint32(exifData[off+4 : off+8])
	return num, den
}

// formatExposureTime formats a rational exposure time as "1/250" or "0.5000".
func formatExposureTime(num, den uint32) string {
	if den == 0 {
		return ""
	}
	if num == 1 {
		return fmt.Sprintf("1/%d", den)
	}
	val := float64(num) / float64(den)
	if val >= 1 {
		return fmt.Sprintf("%.2f", val)
	}
	return fmt.Sprintf("%.4f", val)
}
