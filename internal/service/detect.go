package service

import (
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
	"io"
	"os"
	"strings"
	"time"

	"github.com/richardwooding/c2pa"
)

// TC260 field keys.
const ContentProducerKey = "ContentProducer"

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
