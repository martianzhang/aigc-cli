package sfx

import (
	"math"
	"math/rand/v2"
)

// schedule returns the rectified-flow timestep schedule:
// linspace(sigmaMax, 0, steps+1). sigmaMax < 1 starts from less noise.
func schedule(steps int, sigmaMax float32) []float32 {
	if steps < 1 {
		steps = 1
	}
	if sigmaMax <= 0 || sigmaMax > 1 {
		sigmaMax = 1
	}
	out := make([]float32, steps+1)
	for i := range out {
		out[i] = sigmaMax * (1 - float32(i)/float32(steps))
	}
	return out
}

// pingPong runs the distilled pingpong sampler. velocity must return the
// model's velocity prediction for (x, t); the returned slice is consumed
// immediately and may alias an internal buffer.
//
// Each step produces a denoised estimate and re-injects fresh Gaussian noise at
// the next timestep, matching stable-audio-tools' sample_flow_pingpong:
//
//	denoised = x - t*v
//	x = (1-t_next)*denoised + t_next*noise
func pingPong(x []float32, sched []float32, rng *rand.Rand, velocity func(t float32, x []float32) []float32) []float32 {
	out := append([]float32(nil), x...)
	if len(sched) < 2 {
		return out
	}
	steps := len(sched) - 1
	if len(out) == 0 {
		return out
	}

	noise := make([]float32, len(out))
	for i := 0; i < steps; i++ {
		tCurr := sched[i]
		tNext := sched[i+1]
		v := velocity(tCurr, out)
		for j := range noise {
			noise[j] = float32(rng.NormFloat64())
		}
		for j := range out {
			denoised := out[j] - tCurr*v[j]
			out[j] = (1-tNext)*denoised + tNext*noise[j]
		}
	}
	return out
}

// peakNormalize scales samples in place so the absolute peak equals target.
// Silent input is left untouched.
func peakNormalize(samples []float32, target float32) {
	if target <= 0 {
		return
	}
	var peak float32
	for _, s := range samples {
		if a := float32(math.Abs(float64(s))); a > peak {
			peak = a
		}
	}
	if peak == 0 {
		return
	}
	gain := target / peak
	for i := range samples {
		samples[i] *= gain
	}
}

// clampSamples clips interleaved samples to [-1, 1].
func clampSamples(samples []float32) {
	for i, s := range samples {
		if s > 1 {
			samples[i] = 1
		} else if s < -1 {
			samples[i] = -1
		}
	}
}
