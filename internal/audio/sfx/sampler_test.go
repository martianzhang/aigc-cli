package sfx

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestSchedule(t *testing.T) {
	got := schedule(8, 1)
	if len(got) != 9 {
		t.Fatalf("len = %d, want 9", len(got))
	}
	if got[0] != 1 || got[8] != 0 {
		t.Errorf("endpoints = %v..%v, want 1..0", got[0], got[8])
	}
	if got[4] != 0.5 {
		t.Errorf("midpoint = %v, want 0.5", got[4])
	}
	if schedule(0, 1)[0] != 1 {
		t.Error("steps<1 should still return a usable schedule")
	}
}

func TestPingPongReachesDenoised(t *testing.T) {
	// A velocity of x means denoised = x - t*x = (1-t)x, which decays to zero
	// as t approaches 1 and stays at zero at t=0. Output must be finite.
	initial := make([]float32, 16)
	for i := range initial {
		initial[i] = 1
	}
	rng := rand.New(rand.NewPCG(1, 2))
	out := pingPong(initial, schedule(8, 1), rng, func(_ float32, x []float32) []float32 {
		v := make([]float32, len(x))
		copy(v, x)
		return v
	})
	for i, s := range out {
		if math.IsNaN(float64(s)) || math.IsInf(float64(s), 0) {
			t.Fatalf("sample %d is not finite: %v", i, s)
		}
	}
}

func TestPingPongDeterministic(t *testing.T) {
	initial := []float32{0.1, -0.2, 0.3}
	velocity := func(_ float32, x []float32) []float32 {
		v := make([]float32, len(x))
		for i := range x {
			v[i] = x[i] * 0.5
		}
		return v
	}

	run := func() []float32 {
		rng := rand.New(rand.NewPCG(7, 11))
		return pingPong(initial, schedule(8, 1), rng, velocity)
	}
	a, b := run(), run()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d differs: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestScheduleSigmaMax(t *testing.T) {
	got := schedule(4, 0.5)
	if got[0] != 0.5 || got[4] != 0 {
		t.Errorf("schedule = %v, want 0.5..0", got)
	}
	if got := schedule(4, 2); got[0] != 1 {
		t.Errorf("out-of-range sigmaMax should fall back to 1, got %v", got[0])
	}
}

func TestPeakNormalize(t *testing.T) {
	samples := []float32{0.25, -0.5, 0.1}
	peakNormalize(samples, 0.5)
	if got := maxAbs(samples); math.Abs(float64(got-0.5)) > 1e-6 {
		t.Errorf("peak = %v, want 0.5", got)
	}

	silent := []float32{0, 0}
	peakNormalize(silent, 0.5)
	if silent[0] != 0 || silent[1] != 0 {
		t.Error("silent input must stay silent")
	}

	peakNormalize(samples, 0)
	if got := maxAbs(samples); got != 0.5 {
		t.Errorf("target 0 should be a no-op, peak = %v", got)
	}
}

func TestClampSamples(t *testing.T) {
	samples := []float32{2, -3, 0.5}
	clampSamples(samples)
	if samples[0] != 1 || samples[1] != -1 || samples[2] != 0.5 {
		t.Errorf("clamp = %v", samples)
	}
}

func maxAbs(samples []float32) float32 {
	var peak float32
	for _, s := range samples {
		if a := float32(math.Abs(float64(s))); a > peak {
			peak = a
		}
	}
	return peak
}
