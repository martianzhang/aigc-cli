// Package sfx provides local text-to-sound-effect generation with ONNX
// (Stable Audio Open Small). Inference is pure Go through pure-onnx; no CGO
// and no Python runtime is required.
package sfx

import "fmt"

const (
	// SampleRate is the model's output sample rate.
	SampleRate = 44100
	// Channels is the model's output channel count (stereo).
	Channels = 2
	// TextTokens is the fixed T5 sequence length.
	TextTokens = 64
	// TextDim is the T5 hidden size.
	TextDim = 768
	// LatentDim is the DiT latent channel count.
	LatentDim = 64
	// LatentFrames is the fixed latent length (256 frames).
	LatentFrames = 256
	// AudioFrames is the decoded sample count per channel (11.89s).
	AudioFrames = 524288
	// Steps is the distilled pingpong sampler step count.
	Steps = 8
	// TargetPeak is the default normalization ceiling (-1 dBFS).
	TargetPeak = 0.891251
)

// MaxSeconds is the longest clip the model can produce.
const MaxSeconds = float64(AudioFrames) / float64(SampleRate)

// RequiredFiles lists the model bundle files, relative to the model directory.
var RequiredFiles = []string{
	"dit_step.onnx",
	"dit_step.onnx.data",
	"t5_text_encoder.onnx",
	"t5_text_encoder.onnx.data",
	"vae_decoder.onnx",
	"vae_decoder.onnx.data",
	"t5_tokenizer_json.json",
	"seconds_embedder.json",
}

// Request describes one sound-effect generation.
type Request struct {
	// Prompt is the sound description (English works best).
	Prompt string
	// Seconds is the requested duration, validated against MaxSeconds.
	Seconds float64
	// Seed makes generation deterministic. Zero picks a time-based seed.
	Seed int64
	// Normalize scales the output so its peak sits at TargetPeak.
	Normalize bool
}

// Audio is generated interleaved stereo PCM in [-1, 1].
type Audio struct {
	Samples    []float32
	SampleRate int
	Channels   int
	Seed       int64
}

// Duration returns the clip length in seconds.
func (a *Audio) Duration() float64 {
	if a == nil || a.Channels == 0 {
		return 0
	}
	return float64(len(a.Samples)/a.Channels) / float64(a.SampleRate)
}

// Validate checks the request against the model's limits.
func (r Request) Validate() error {
	if r.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}
	if r.Seconds <= 0 {
		return fmt.Errorf("duration must be positive")
	}
	if r.Seconds > MaxSeconds {
		return fmt.Errorf("duration %.2fs exceeds the model limit of %.2fs", r.Seconds, MaxSeconds)
	}
	return nil
}
