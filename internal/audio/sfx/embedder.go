package sfx

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
)

// secondsEmbedder maps a duration in seconds to the 768-dim conditioning
// vector used by both the DiT cross-attention token and its global condition.
//
// It reproduces stable-audio-tools' NumberConditioner:
//
//	x = clamp(seconds, min, max) normalized to [0,1]
//	features = [x, sin(2πxw), cos(2πxw)]
//	embed = features @ Wᵀ + b
type secondsEmbedder struct {
	MinVal        float64     `json:"min_val"`
	MaxVal        float64     `json:"max_val"`
	FourierWeight []float64   `json:"fourier_weights"`
	LinearWeight  [][]float64 `json:"linear_weight"`
	LinearBias    []float64   `json:"linear_bias"`
}

func loadSecondsEmbedder(path string) (*secondsEmbedder, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read seconds embedder: %w", err)
	}
	var s secondsEmbedder
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse seconds embedder: %w", err)
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *secondsEmbedder) validate() error {
	if s.MaxVal <= s.MinVal {
		return fmt.Errorf("seconds embedder range is invalid: min=%v max=%v", s.MinVal, s.MaxVal)
	}
	if len(s.FourierWeight) != 128 {
		return fmt.Errorf("seconds embedder fourier weights: got %d, want 128", len(s.FourierWeight))
	}
	if len(s.LinearBias) != TextDim {
		return fmt.Errorf("seconds embedder bias: got %d, want %d", len(s.LinearBias), TextDim)
	}
	if len(s.LinearWeight) != TextDim {
		return fmt.Errorf("seconds embedder weight rows: got %d, want %d", len(s.LinearWeight), TextDim)
	}
	cols := 1 + len(s.FourierWeight)*2
	for i, row := range s.LinearWeight {
		if len(row) != cols {
			return fmt.Errorf("seconds embedder weight row %d: got %d cols, want %d", i, len(row), cols)
		}
	}
	return nil
}

// embed returns the conditioning vector for a duration in seconds.
func (s *secondsEmbedder) embed(seconds float64) []float32 {
	v := math.Min(math.Max(seconds, s.MinVal), s.MaxVal)
	x := (v - s.MinVal) / (s.MaxVal - s.MinVal)

	half := len(s.FourierWeight)
	features := make([]float64, 1+half*2)
	features[0] = x
	for i, w := range s.FourierWeight {
		f := 2 * math.Pi * x * w
		features[1+i] = math.Sin(f)
		features[1+half+i] = math.Cos(f)
	}

	out := make([]float32, len(s.LinearBias))
	for j := range out {
		acc := s.LinearBias[j]
		row := s.LinearWeight[j]
		for k, f := range features {
			acc += row[k] * f
		}
		out[j] = float32(acc)
	}
	return out
}
