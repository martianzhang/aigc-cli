package vision

// KVCache manages key-value cache for autoregressive decoder inference.
//
// Florence-2's decoder has both self-attention KV cache and cross-attention
// KV cache. The cross-attention KV is computed once by the encoder and reused
// for each decoder step. The self-attention KV grows with each generated token.
type KVCache struct {
	// SelfAttention holds the cached key/value tensors for self-attention layers.
	// Each entry is a flat float32 slice representing one KV tensor.
	SelfAttention map[string][]float32

	// CrossAttention holds the encoder output used as KV in cross-attention.
	// This is computed once by Encode and reused across all decoder steps.
	CrossAttention []float32

	// Step is the current generation step (number of tokens generated so far).
	Step int

	// NumLayers is the number of decoder layers.
	NumLayers int

	// HeadDim is the hidden dimension per attention head.
	HeadDim int
}

// NewKVCache creates an empty KV cache.
func NewKVCache(numLayers, headDim int) *KVCache {
	return &KVCache{
		SelfAttention: make(map[string][]float32),
		Step:          0,
		NumLayers:     numLayers,
		HeadDim:       headDim,
	}
}

// Reset clears the cache for a new inference sequence.
func (c *KVCache) Reset() {
	c.SelfAttention = make(map[string][]float32)
	c.CrossAttention = nil
	c.Step = 0
}

// SetCrossAttention stores the encoder output for cross-attention.
func (c *KVCache) SetCrossAttention(encoderStates []float32) {
	c.CrossAttention = encoderStates
}

// GetCrossAttention returns the cached encoder states.
func (c *KVCache) GetCrossAttention() []float32 {
	return c.CrossAttention
}

// SetSelfAttention stores a self-attention KV tensor for a given layer name.
func (c *KVCache) SetSelfAttention(name string, tensor []float32) {
	c.SelfAttention[name] = tensor
}

// GetSelfAttention retrieves a self-attention KV tensor.
func (c *KVCache) GetSelfAttention(name string) []float32 {
	return c.SelfAttention[name]
}

// HasSelfAttention checks if a self-attention KV tensor exists.
func (c *KVCache) HasSelfAttention(name string) bool {
	_, ok := c.SelfAttention[name]
	return ok
}

// IncrementStep advances the generation step counter.
func (c *KVCache) IncrementStep() {
	c.Step++
}

// AttentionMaskSuffix returns the attention mask suffix for the current step.
// For the first step (prompt), the mask allows full attention.
// For subsequent steps (autoregressive), the mask is causal.
func (c *KVCache) AttentionMaskSuffix(promptLen int) []float32 {
	totalLen := promptLen + c.Step
	mask := make([]float32, totalLen)
	for i := 0; i < totalLen; i++ {
		if i <= promptLen+c.Step-1 {
			mask[i] = 0.0 // allowed
		} else {
			mask[i] = -65504.0 // masked (float16 min)
		}
	}
	return mask
}
