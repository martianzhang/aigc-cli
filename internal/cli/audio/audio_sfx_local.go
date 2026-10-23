package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/audio"
	sfxengine "github.com/martianzhang/aigc-cli/internal/audio/sfx"
	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// checkLocalSFXFlags rejects online-only flags in local mode.
func checkLocalSFXFlags(cmd *cobra.Command) error {
	if sfxLoop {
		return fmt.Errorf("--loop is not supported by the local provider (use the online ElevenLabs provider)")
	}
	if cmd.Flags().Changed("influence") {
		return fmt.Errorf("--influence is not supported by the local provider (use the online ElevenLabs provider)")
	}
	if sfxFormat != "" && sfxFormat != "wav" {
		return fmt.Errorf("the local provider produces wav only, got --format %q", sfxFormat)
	}
	return nil
}

// checkOnlineSFXFlags rejects local-only flags in online mode.
func checkOnlineSFXFlags(cmd *cobra.Command) error {
	if cmd.Flags().Changed("seed") {
		return fmt.Errorf("--seed only applies to -P local; online providers do not accept a seed")
	}
	if cmd.Flags().Changed("no-normalize") {
		return fmt.Errorf("--no-normalize only applies to -P local")
	}
	return nil
}

// runLocalSFX generates a sound effect with the local ONNX model. The model ID
// comes from --model, then defaults.audio.sfx.model, then the built-in default.
func runLocalSFX(prompt string, p *provider.EffectiveProvider) error {
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
	if _, err := os.Stat(modelDir); err != nil {
		return fmt.Errorf("model %q not found at %s\nRun 'aigc-cli audio init --model %s' to download it", modelID, modelDir, modelID)
	}
	libPath, err := onnxrt.LibPath(modelsRoot)
	if err != nil {
		return fmt.Errorf("%w\nRun 'aigc-cli audio init --model %s' to install ONNX Runtime", err, modelID)
	}

	seconds := sfxDuration
	if seconds == 0 {
		seconds = 5
	}
	req := sfxengine.Request{
		Prompt:    prompt,
		Seconds:   seconds,
		Seed:      sfxSeed,
		Normalize: !sfxNoNormalize,
	}
	if err := req.Validate(); err != nil {
		return err
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
