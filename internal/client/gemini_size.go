package client

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// geminiAspectRatios lists the aspect ratios Gemini image models accept. The
// list comes from Google's image-generation docs; a value outside it is sent
// through unchanged so the API can reject it with its own message instead of
// the CLI silently swapping in 1:1.
var geminiAspectRatios = [][2]int{
	{1, 1}, {1, 4}, {1, 8}, {2, 3}, {3, 2}, {3, 4}, {4, 1},
	{4, 3}, {4, 5}, {5, 4}, {8, 1}, {9, 16}, {16, 9}, {21, 9},
}

var (
	geminiTierPattern  = regexp.MustCompile(`^(\d+(?:\.\d+)?)k$`)
	geminiPixelPattern = regexp.MustCompile(`^(\d+)\s*[xX*]\s*(\d+)$`)
	geminiRatioPattern = regexp.MustCompile(`^(\d+)\s*:\s*(\d+)$`)
)

// geminiImageFormat maps the CLI's --size/--ratio values onto Gemini's two
// sizing fields. Gemini has no single "size" parameter: the aspect ratio and
// the resolution tier are independent inputs.
//
//   - image_size comes from a tier ("2K", "4K", "0.5K", "512").
//   - aspect_ratio prefers --ratio, then a ratio --size ("16:9"), then a pixel
//     --size ("2048x1152") reduced to its simplest form and matched against
//     the ratios Gemini supports.
//   - "auto" and anything unrecognized leave the field unset, so the model
//     keeps its own default.
//
// Unknown tiers ("3K") pass through verbatim: Gemini reports the rejection
// itself, which is clearer than dropping the value.
func geminiImageFormat(size, ratio string) (aspectRatio, imageSize string) {
	size = strings.TrimSpace(size)
	ratio = strings.TrimSpace(ratio)

	if tier, ok := geminiTier(size); ok {
		imageSize = tier
	}
	if isAutoSize(ratio) {
		ratio = ""
	}
	if ratio != "" {
		if parsed, ok := geminiRatio(ratio); ok {
			return parsed, imageSize
		}
	}
	if isAutoSize(size) {
		return "", imageSize
	}
	if parsed, ok := geminiRatio(size); ok {
		return parsed, imageSize
	}
	if w, h, ok := geminiPixels(size); ok {
		return geminiNearestRatio(w, h), imageSize
	}
	return "", imageSize
}

// geminiTier reports whether v is a resolution tier and returns it with an
// uppercase K. "512" and "512px" are the pixel-spelled form of Gemini's lowest
// tier.
func geminiTier(v string) (string, bool) {
	lower := strings.ToLower(v)
	switch lower {
	case "512", "512px":
		return "512", true
	}
	m := geminiTierPattern.FindStringSubmatch(lower)
	if m == nil {
		return "", false
	}
	return m[1] + "K", true
}

// geminiRatio parses "A:B" and returns it unchanged apart from whitespace.
func geminiRatio(v string) (string, bool) {
	m := geminiRatioPattern.FindStringSubmatch(v)
	if m == nil {
		return "", false
	}
	w, _ := strconv.Atoi(m[1])
	h, _ := strconv.Atoi(m[2])
	if w <= 0 || h <= 0 {
		return "", false
	}
	return m[1] + ":" + m[2], true
}

// geminiPixels parses "WxH" (also "WXH" and "W*H", the DashScope spelling).
func geminiPixels(v string) (w, h int, ok bool) {
	m := geminiPixelPattern.FindStringSubmatch(v)
	if m == nil {
		return 0, 0, false
	}
	w, errW := strconv.Atoi(m[1])
	h, errH := strconv.Atoi(m[2])
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

// geminiNearestRatio reduces W:H and returns the closest supported ratio. The
// distance is measured on log(a/b) so 2:3 and 3:2 are symmetric around 1:1.
func geminiNearestRatio(w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	g := gcd(w, h)
	rw, rh := w/g, h/g
	target := math.Log(float64(rw) / float64(rh))

	best := ""
	bestDistance := math.Inf(1)
	for _, r := range geminiAspectRatios {
		distance := math.Abs(target - math.Log(float64(r[0])/float64(r[1])))
		if distance < bestDistance {
			best, bestDistance = fmt.Sprintf("%d:%d", r[0], r[1]), distance
		}
	}
	return best
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// isAutoSize reports whether v asks the model to pick the size.
func isAutoSize(v string) bool {
	return strings.EqualFold(strings.TrimSpace(v), "auto")
}
