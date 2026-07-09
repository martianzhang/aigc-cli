// Package watermark provides visible AI watermark detection and removal.
// Core engine: coarse-to-fine NCC template matching + reverse alpha blending.
// Platform-specific configs (alpha maps, position) are registered separately.
package watermark

// Type identifies a known visible watermark pattern.
type Type int

const (
	TypeUnknown Type = iota
	TypeGeminiSparkle
	TypeDoubao
	TypeJimeng
	TypeDoubaoSnap // "AI 生成" UI badge (screenshot from Doubao web)
	TypeBaidu      // "百度 AI生成" embedded text watermark
	TypeZhipu      // "智谱清言" embedded text watermark (Zhipu Qingyan / ChatGLM)
)

// RemoveStrategy controls how a watermark is removed.
type RemoveStrategy int

const (
	RemoveAlphaBlend RemoveStrategy = iota // reverse alpha blending (default)
	RemoveInpaint                          // inpaint only (UI badges with opaque background)
	RemoveSkip                             // detection only, skip removal
)

// AlphaMap holds a pre-calibrated transparency mask for a watermark.
// Data is row-major float64 in [0, 1], where 1 = fully opaque watermark.
type AlphaMap struct {
	Width  int
	Height int
	Data   []float64
}

// NewAlphaMap creates an AlphaMap from a flat float64 slice.
func NewAlphaMap(w, h int, data []float64) *AlphaMap {
	return &AlphaMap{Width: w, Height: h, Data: data}
}

// At returns the alpha value at (x, y) relative to the watermark origin.
func (am *AlphaMap) At(x, y int) float64 {
	if x < 0 || x >= am.Width || y < 0 || y >= am.Height {
		return 0
	}
	return am.Data[y*am.Width+x]
}

// Position describes a candidate watermark location.
type Position struct {
	X, Y, W, H int // position and dimensions (W=H for square alpha maps)
}

// Size returns the square size or min dimension for NCC matching.
func (p Position) Size() int {
	if p.W == p.H {
		return p.W
	}
	if p.W < p.H {
		return p.W
	}
	return p.H
}

// PositionResolver computes candidate watermark positions for a given image size.
// Used by text-based watermarks (Doubao, Jimeng) that scale with image dimensions.
type PositionResolver func(w, h int) []Position

// Config describes one watermark type's detection parameters and alpha map.
type Config struct {
	Type           Type
	Name           string
	AlphaMap       *AlphaMap
	DefaultSize    int // default watermark size in px
	DefaultMarginX int // default right/bottom margin in px
	DefaultMarginY int
	LogoColor      [3]float64 // RGB of watermark color (255 for white)
	// Detection thresholds
	DetectThreshold float64 // minimum NCC score to consider detected
	// Size search range (fraction of default)
	MinSizeScale float64 // default 0.5
	MaxSizeScale float64 // default 2.0
	// Margin search range (absolute px offset from default)
	MarginRange int // default 48
	// PositionResolver overrides DefaultSize/Margin with image-relative positions.
	// When set, DefaultSize and DefaultMargin* serve as fallback only.
	PositionResolver PositionResolver
	// RemoveStrategy controls how the watermark is removed.
	// Default (RemoveAlphaBlend) uses reverse alpha blending.
	// Set RemoveInpaint for UI badges with opaque backgrounds.
	RemoveStrategy RemoveStrategy
	// OversubtractMargin is the over-subtraction guard threshold in gray
	// levels: when reverse-alpha would darken the glyph body more than this
	// below the surrounding background ring, fall back to inpainting. 0 means
	// use the default (25). This is data-driven per producer so it can be
	// tuned (or emitted by the alpha-map generator) for each watermark type.
	OversubtractMargin float64
}

// Result holds the outcome of a watermark removal operation.
type Result struct {
	Removed    bool    `json:"removed"`
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
	AlphaGain  float64 `json:"alpha_gain"`
	Size       int     `json:"size"`
	Region     string  `json:"region"`
}

// Detection holds the outcome of watermark detection.
type Detection struct {
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
	X          int     `json:"x"`
	Y          int     `json:"y"`
	Size       int     `json:"size"`
	W          int     `json:"w"` // width of detected watermark (0 for square)
	H          int     `json:"h"` // height of detected watermark (0 for square)
}

// registry holds all registered watermark configs.
var registry []Config

// Register adds a watermark config to the registry.
func Register(cfg Config) {
	registry = append(registry, cfg)
}

// RegisteredTypes returns all registered type names.
func RegisteredTypes() []string {
	names := make([]string, len(registry))
	for i, c := range registry {
		names[i] = c.Name
	}
	return names
}

// findConfigByName returns the config with the given name.
func findConfigByName(name string) (Config, bool) {
	for _, c := range registry {
		if c.Name == name {
			return c, true
		}
	}
	return Config{}, false
}

// clampByte clamps a float64 to [0, 255] uint8.
func clampByte(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
