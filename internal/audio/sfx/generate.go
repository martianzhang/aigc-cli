package sfx

import (
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

// Generate runs the full text-to-audio pipeline: tokenize, condition, pingpong
// sample, decode, trim and optionally normalize.
func (e *Engine) Generate(req Request) (*Audio, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	seed := req.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil, fmt.Errorf("engine is closed")
	}

	ids, mask, err := e.tokenizer.Encode(req.Prompt)
	if err != nil {
		return nil, err
	}

	copy(e.idsIn.GetData(), ids)
	copy(e.maskIn.GetData(), mask)
	if err := e.textSess.Run(); err != nil {
		return nil, fmt.Errorf("text encoder: %w", err)
	}

	emb := e.textOut.GetData()
	for i := 0; i < TextTokens; i++ {
		if mask[i] != 0 {
			continue
		}
		for j := 0; j < TextDim; j++ {
			emb[i*TextDim+j] = 0
		}
	}

	seconds := e.embedder.embed(req.Seconds)
	cond := e.condIn.GetData()
	copy(cond[:TextTokens*TextDim], emb)
	copy(cond[TextTokens*TextDim:], seconds)
	copy(e.globalIn.GetData(), seconds)

	steps := req.Steps
	if steps <= 0 {
		steps = Steps
	}
	sigmaMax := float32(req.SigmaMax)
	if sigmaMax <= 0 {
		sigmaMax = 1
	}
	sched := schedule(steps, sigmaMax)

	rng := rand.New(rand.NewPCG(uint64(seed), 0x9e3779b97f4a7c15))
	x := e.xIn.GetData()
	for i := range x {
		x[i] = float32(rng.NormFloat64())
	}

	var stepErr error
	latent := pingPong(x, sched, rng, func(t float32, in []float32) []float32 {
		if stepErr != nil {
			return make([]float32, len(in))
		}
		copy(e.xIn.GetData(), in)
		e.tIn.GetData()[0] = t
		if err := e.ditSess.Run(); err != nil {
			stepErr = err
			return make([]float32, len(in))
		}
		return e.vOut.GetData()
	})
	if stepErr != nil {
		return nil, fmt.Errorf("diffusion step: %w", stepErr)
	}

	copy(e.latIn.GetData(), latent)
	if err := e.vaeSess.Run(); err != nil {
		return nil, fmt.Errorf("vae decode: %w", err)
	}

	frames := int(math.Round(req.Seconds * SampleRate))
	if frames > AudioFrames {
		frames = AudioFrames
	}
	if frames < 1 {
		frames = 1
	}
	raw := e.audioOut.GetData()
	samples := make([]float32, frames*Channels)
	for i := 0; i < frames; i++ {
		samples[i*Channels] = raw[i]
		samples[i*Channels+1] = raw[AudioFrames+i]
	}
	if req.Normalize {
		peakNormalize(samples, TargetPeak)
	}
	clampSamples(samples)

	return &Audio{
		Samples:    samples,
		SampleRate: SampleRate,
		Channels:   Channels,
		Seed:       seed,
	}, nil
}
