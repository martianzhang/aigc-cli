package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/martianzhang/aigc-cli/internal/forensic"
	"github.com/martianzhang/aigc-cli/internal/onnx"
	"github.com/martianzhang/aigc-cli/internal/wmremove"
)

var wmDetector *wmremove.Detector

// detectOnce lazily initializes the ONNX detector and caches the instance
// for reuse across detect files, avoiding repeated model loading.
var detectOnce struct {
	sync.Once
	detector *onnx.Detector
}

func detectFiles(paths []string, pathOverride string) error {
	if detectOnce.detector == nil {
		detectOnce.Do(func() {
			detectOnce.detector = tryInitONNX()
		})
	}
	aiDetector := detectOnce.detector
	if aiDetector != nil {
		defer aiDetector.Close()
	}
	if detectRemoveWM {
		if d, err := tryInitWMRemove(); err == nil {
			wmDetector = d
		}
	}

	if detectJSON {
		return detectFilesJSON(paths, pathOverride, aiDetector)
	}

	for _, path := range paths {
		if err := detectOneFile(path, pathOverride, aiDetector); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
	return nil
}

// buildDetails creates a compact breakdown of all signals.
func buildDetails(r *forensic.Result) string {
	s := ""
	for _, sig := range r.Signals {
		if s != "" {
			s += "; "
		}
		s += fmt.Sprintf("%s=%.0f%%", sig.Name, sig.Score*100)
	}
	return s
}

// parseLLMScore extracts a 0-1 score from the LLM's response text.
// The LLM is asked to output a number 0-100, so we look for it.
func parseLLMScore(text string) float64 {
	// Scan for "N/100" or "N out of 100" pattern
	parts := strings.Fields(text)
	for i, p := range parts {
		cleaned := strings.TrimRight(p, ".,!?%")
		if n, err := strconv.Atoi(cleaned); err == nil && n >= 0 && n <= 100 {
			// Check it's not followed by a year or other non-score number
			if i+1 < len(parts) && parts[i+1] == "/100" {
				return float64(n) / 100.0
			}
			if strings.Contains(p, "/100") || strings.Contains(p, "%") {
				return float64(n) / 100.0
			}
		}
	}
	// Fallback: keyword-based heuristic
	lower := strings.ToLower(text)
	aiIndicators := []string{"ai-generated", "likely ai", "artificial", "synthetic", "deepfake", "generated"}
	humanIndicators := []string{"human-made", "real photo", "natural", "authentic", "realistic"}
	aiScore := 0.0
	for _, kw := range aiIndicators {
		if strings.Contains(lower, kw) {
			aiScore += 0.3
		}
	}
	for _, kw := range humanIndicators {
		if strings.Contains(lower, kw) {
			aiScore -= 0.3
		}
	}
	aiScore = max(0, min(1, aiScore+0.5))
	return aiScore
}
