package vision

import "math"

// Sampler handles token sampling strategies for autoregressive generation.
type Sampler struct {
	Temperature float64
	TopK        int
}

// DefaultSampler returns a sampler with greedy decoding.
func DefaultSampler() *Sampler {
	return &Sampler{
		Temperature: 0.0, // greedy
		TopK:        0,
	}
}

// NewSampler creates a sampler with the given parameters.
// temperature=0 means greedy (always pick highest probability).
func NewSampler(temperature float64, topK int) *Sampler {
	return &Sampler{
		Temperature: temperature,
		TopK:        topK,
	}
}

// Sample selects the next token ID from logits.
// When temperature is 0 (or very small), uses greedy selection.
func (s *Sampler) Sample(logits []float32) int64 {
	if s.Temperature <= 0.001 || len(logits) == 0 {
		return s.greedy(logits)
	}
	return s.topKLogits(logits)
}

// greedy selects the token with the highest logit value.
func (s *Sampler) greedy(logits []float32) int64 {
	bestIdx := 0
	bestVal := logits[0]
	for i, v := range logits[1:] {
		if v > bestVal {
			bestVal = v
			bestIdx = i + 1
		}
	}
	return int64(bestIdx)
}

// topKLogits applies temperature scaling and top-k filtering, then samples.
func (s *Sampler) topKLogits(logits []float32) int64 {
	n := len(logits)
	scaled := make([]float64, n)

	// Apply temperature scaling
	temp := s.Temperature
	if temp < 0.001 {
		temp = 0.001
	}
	for i, v := range logits {
		scaled[i] = float64(v) / temp
	}

	// Apply softmax
	maxVal := scaled[0]
	for _, v := range scaled[1:] {
		if v > maxVal {
			maxVal = v
		}
	}
	var sum float64
	for i, v := range scaled {
		scaled[i] = math.Exp(v - maxVal)
		sum += scaled[i]
	}
	for i := range scaled {
		scaled[i] /= sum
	}

	// Top-K filtering
	k := s.TopK
	if k <= 0 || k > n {
		k = n
	}

	// Find top-k threshold
	threshold := getKthLargest(scaled, k)

	// Sample from filtered distribution
	var filteredSum float64
	filteredProbs := make([]float64, n)
	for i, p := range scaled {
		if p >= threshold {
			filteredProbs[i] = p
			filteredSum += p
		}
	}

	if filteredSum <= 0 {
		return s.greedy(logits)
	}

	// Normalize and sample
	r := math.Float64frombits(0x3FF0000000000000) // not used for greedy
	_ = r                                         // placeholder for actual random sampling (currently greedy within top-k)

	// Pick highest probability among top-k (deterministic)
	bestIdx := 0
	bestVal := float64(0)
	for i, p := range filteredProbs {
		if p > bestVal {
			bestVal = p
			bestIdx = i
		}
	}
	return int64(bestIdx)
}

// getKthLargest returns the k-th largest value in the slice.
// Uses a simple approach since n is small (vocab size ~50k).
func getKthLargest(probs []float64, k int) float64 {
	if k <= 0 || k > len(probs) {
		return 0
	}
	sorted := make([]float64, len(probs))
	copy(sorted, probs)
	// Partial sort: find k-th largest
	for i := 0; i < k && i < len(sorted); i++ {
		maxIdx := i
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] > sorted[maxIdx] {
				maxIdx = j
			}
		}
		sorted[i], sorted[maxIdx] = sorted[maxIdx], sorted[i]
	}
	return sorted[k-1]
}
