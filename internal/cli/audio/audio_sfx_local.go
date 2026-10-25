package audio

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/martianzhang/aigc-cli/internal/audio"
	sfxengine "github.com/martianzhang/aigc-cli/internal/audio/sfx"
	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// checkLocalSFXFlags rejects online-only flags in local mode.
func checkLocalSFXFlags() error {
	if sfxFormat != "" && sfxFormat != "wav" {
		return fmt.Errorf("the local provider produces wav only, got --format %q", sfxFormat)
	}
	return nil
}

// runLocalSFX generates a sound effect with the local ONNX model. The model ID
// comes from --model, then defaults.audio.sfx.model, then the built-in default.
func runLocalSFX(prompt string, p *provider.EffectiveProvider, inject json.RawMessage) error {
	modelID := sfxModel
	if modelID == "" && p != nil {
		modelID = p.Model
	}
	if modelID == "" {
		modelID = audio.SFXDefaultModelID
	}

	modelsRoot := filepath.Dir(audioModelsDir())
	modelDir := filepath.Join(audioModelsDir(), modelID)
	if filepath.IsAbs(modelID) {
		// Escape hatch: --model /path/to/bundle for development and tests.
		modelDir = modelID
	}

	seconds := sfxDuration
	if seconds == 0 {
		seconds = 5
	}
	tuning, err := sfxengine.ParseTuning(inject)
	if err != nil {
		return err
	}
	seed := int64(0)
	if tuning.Seed != nil {
		seed = *tuning.Seed
	}
	normalize := true
	if tuning.Normalize != nil {
		normalize = *tuning.Normalize
	}
	req := sfxengine.Request{
		Prompt:    prompt,
		Seconds:   seconds,
		Seed:      seed,
		Steps:     tuning.Steps,
		SigmaMax:  tuning.SigmaMax,
		Normalize: normalize,
	}
	if err := req.Validate(); err != nil {
		return err
	}

	if audioDryRun {
		seed := "random"
		if tuning.Seed != nil {
			seed = strconv.FormatInt(*tuning.Seed, 10)
		}
		steps := tuning.Steps
		if steps == 0 {
			steps = 8 // distilled default
		}
		sigmaMax := tuning.SigmaMax
		if sigmaMax == 0 {
			sigmaMax = 1
		}
		printLocalDryRun("audio sfx", []localDryRunField{
			{"model", modelID},
			{"prompt", fmt.Sprintf("%d chars", len(prompt))},
			{"duration", fmt.Sprintf("%.2fs", seconds)},
			{"seed", seed},
			{"steps", strconv.Itoa(steps)},
			{"sigma_max", fmt.Sprintf("%g", sigmaMax)},
			{"normalize", strconv.FormatBool(normalize)},
			{"output", plannedAudioOutput("wav")},
		})
		return nil
	}

	if _, err := os.Stat(modelDir); err != nil {
		return fmt.Errorf("model %q not found at %s\nRun 'aigc-cli audio init --model %s' to download it", modelID, modelDir, modelID)
	}
	libPath, err := onnxrt.LibPath(modelsRoot)
	if err != nil {
		return fmt.Errorf("%w\nRun 'aigc-cli audio init --model %s' to install ONNX Runtime", err, modelID)
	}

	engine, err := sfxengine.NewEngine(modelDir, libPath)
	if err != nil {
		return fmt.Errorf("load sfx model: %w", err)
	}
	defer engine.Close()

	start := time.Now()
	generated, err := engine.Generate(req)
	if err != nil {
		return fmt.Errorf("sound effect failed: %w", err)
	}
	elapsed := time.Since(start)

	wav, err := sfxengine.EncodeWAV(generated.Samples, generated.Channels, generated.SampleRate)
	if err != nil {
		return fmt.Errorf("encode wav: %w", err)
	}
	filename, err := saveAudioFile(wav, "wav")
	if err != nil {
		return fmt.Errorf("failed to save audio: %w", err)
	}

	fmt.Printf("Model: %s (local)\n", modelID)
	fmt.Printf("Format: wav %dHz %dch\n", generated.SampleRate, generated.Channels)
	fmt.Printf("Duration: %.2fs\n", generated.Duration())
	fmt.Printf("Seed: %d\n", generated.Seed)
	fmt.Printf("Inference: %.1fs\n", elapsed.Seconds())
	fmt.Printf("Saved: %s\n", filename)

	if sfxPlay {
		if err := service.PreviewFile(filename); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: playback failed: %v\n", err)
		}
	}
	return nil
}
