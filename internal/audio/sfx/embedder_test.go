package sfx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func syntheticEmbedder() *secondsEmbedder {
	fourier := make([]float64, 128)
	weights := make([][]float64, TextDim)
	bias := make([]float64, TextDim)
	for j := range weights {
		weights[j] = make([]float64, 257)
		bias[j] = float64(j) / TextDim
	}
	return &secondsEmbedder{
		MinVal:        0,
		MaxVal:        256,
		FourierWeight: fourier,
		LinearWeight:  weights,
		LinearBias:    bias,
	}
}

func TestSecondsEmbedderEmbed(t *testing.T) {
	s := syntheticEmbedder()
	out := s.embed(4)
	if len(out) != TextDim {
		t.Fatalf("len = %d, want %d", len(out), TextDim)
	}
	if out[0] != 0 || out[767] != float32(767.0/TextDim) {
		t.Errorf("unexpected embedding: first=%v last=%v", out[0], out[767])
	}
}

func TestSecondsEmbedderClamps(t *testing.T) {
	s := syntheticEmbedder()
	low := s.embed(-10)
	zero := s.embed(0)
	high := s.embed(300)
	max := s.embed(256)
	for i := range low {
		if low[i] != zero[i] {
			t.Fatalf("negative seconds not clamped at index %d", i)
		}
		if high[i] != max[i] {
			t.Fatalf("large seconds not clamped at index %d", i)
		}
	}
}

func TestSecondsEmbedderValidate(t *testing.T) {
	valid := syntheticEmbedder()
	if err := valid.validate(); err != nil {
		t.Fatalf("valid embedder rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*secondsEmbedder)
	}{
		{"bad range", func(s *secondsEmbedder) { s.MaxVal = s.MinVal }},
		{"bad fourier", func(s *secondsEmbedder) { s.FourierWeight = s.FourierWeight[:10] }},
		{"bad bias", func(s *secondsEmbedder) { s.LinearBias = s.LinearBias[:10] }},
		{"bad rows", func(s *secondsEmbedder) { s.LinearWeight = s.LinearWeight[:10] }},
		{"bad cols", func(s *secondsEmbedder) { s.LinearWeight[3] = s.LinearWeight[3][:10] }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := syntheticEmbedder()
			tc.mutate(s)
			if err := s.validate(); err == nil {
				t.Fatal("validate() = nil, want error")
			}
		})
	}
}

func TestLoadSecondsEmbedder(t *testing.T) {
	s := syntheticEmbedder()
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "seconds.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	got, err := loadSecondsEmbedder(path)
	if err != nil {
		t.Fatalf("loadSecondsEmbedder() error = %v", err)
	}
	if len(got.LinearWeight) != TextDim {
		t.Errorf("rows = %d, want %d", len(got.LinearWeight), TextDim)
	}
	if _, err := loadSecondsEmbedder(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("missing file must error")
	}
}
