package cmd

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/audio"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func runAudioTranscribe(cmd *cobra.Command, args []string) error {
	// ── Check local mode: --local flag, type=local, or no provider+model ──
	p := shared.ResolveProvider(ProviderNameAudio)
	if isLocalAudioMode(p, audioTranscribeLocal) {
		return runLocalAudioTranscribe(cmd)
	}

	if audioTranscribeModel == "" {
		if cfg, err := config.LoadDefaults(shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil && cfg.Defaults.Audio != nil && cfg.Defaults.Audio.TranscribeModel != "" {
			audioTranscribeModel = cfg.Defaults.Audio.TranscribeModel
		}
	}
	if audioTranscribeModel == "" {
		audioTranscribeModel = "whisper-1"
	}

	// Auto-detect piped stdin when --input is not specified
	if audioTranscribeInput == "" {
		stat, err := os.Stdin.Stat()
		if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read stdin: %w", err)
			}
			audioTranscribeInput = string(data)
		}
	}
	if audioTranscribeInput == "" {
		return fmt.Errorf("audio input is required: set via --input flag, file path, or stdin")
	}

	c := client.NewFromProvider(p)
	applyTimeout(c, "audio", client.AudioTimeout)

	start := time.Now()

	if isFile(audioTranscribeInput) {
		if shared.Verbose {
			fmt.Printf("Uploading file: %s\n", audioTranscribeInput)
		}

		if audioTranscribeDryRun {
			fmt.Printf("curl %s/audio/transcriptions \\\n", shared.APIBase)
			fmt.Printf("  -H \"Authorization: Bearer %s\" \\\n", maskKey(shared.APIKey))
			fmt.Printf("  -F file=\"@%s\" \\\n", audioTranscribeInput)
			fmt.Printf("  -F model=\"%s\"\n", audioTranscribeModel)
			if audioTranscribeLanguage != "" {
				fmt.Printf("  -F language=\"%s\"\n", audioTranscribeLanguage)
			}
			return nil
		}

		sttResp, err := c.AudioTranscribeMultipart(audioTranscribeModel, audioTranscribeInput, audioTranscribeLanguage)
		if err != nil {
			return fmt.Errorf("STT failed: %w", err)
		}

		elapsed := time.Since(start)
		fmt.Printf("Model: %s\n", audioTranscribeModel)
		fmt.Printf("Duration: %.1fs\n", elapsed.Seconds())
		if sttResp.Usage != nil {
			costStr := ""
			if sttResp.Usage.Cost > 0 {
				costStr = fmt.Sprintf(" | Cost: $%.5f", sttResp.Usage.Cost)
			}
			fmt.Printf("Audio: %.1fs%s\n", sttResp.Usage.Seconds, costStr)
		}

		filename, err := saveTranscriptionFile(sttResp.Text, audioTranscribeInput)
		if err != nil {
			return fmt.Errorf("failed to save transcription: %w", err)
		}
		fmt.Printf("Saved: %s\n", filename)
		return nil
	}

	data, err := readInput(audioTranscribeInput)
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	base64Data := base64.StdEncoding.EncodeToString(data)
	format := audioTranscribeFormat
	if format == "" {
		format = detectAudioFormat(audioTranscribeInput)
	}

	req := &types.AudioTranscribeRequest{
		Model: audioTranscribeModel,
		InputAudio: &types.AudioInput{
			Data:   base64Data,
			Format: format,
		},
		Language:    audioTranscribeLanguage,
		Temperature: audioTranscribeTemperature,
	}

	if audioTranscribeDryRun {
		fmt.Println(buildAudioTranscribeCurl(req))
		return nil
	}

	if shared.Verbose {
		fmt.Printf("Sending base64-encoded %s (%d bytes raw)\n", format, len(data))
	}

	sttResp, err := c.AudioTranscribe(req)
	if err != nil {
		return fmt.Errorf("STT failed: %w", err)
	}

	elapsed := time.Since(start)
	fmt.Printf("Model: %s\n", audioTranscribeModel)
	fmt.Printf("Duration: %.1fs\n", elapsed.Seconds())
	if sttResp.Usage != nil {
		costStr := ""
		if sttResp.Usage.Cost > 0 {
			costStr = fmt.Sprintf(" | Cost: $%.5f", sttResp.Usage.Cost)
		}
		fmt.Printf("Audio: %.1fs%s\n", sttResp.Usage.Seconds, costStr)
	}

	filename, err := saveTranscriptionFile(sttResp.Text, audioTranscribeInput)
	if err != nil {
		return fmt.Errorf("failed to save transcription: %w", err)
	}
	fmt.Printf("Saved: %s\n", filename)
	return nil
}

// runLocalAudioTranscribe handles local ASR inference via sherpa-onnx.
func runLocalAudioTranscribe(cmd *cobra.Command) error {
	input := audioTranscribeInput
	if input == "" {
		return fmt.Errorf("audio file is required: specify with --input")
	}
	if _, err := os.Stat(input); err != nil {
		return fmt.Errorf("file not found: %s", input)
	}

	modelID := audioTranscribeModel
	if modelID == "" {
		if cfg := audioDefaults(); cfg != nil {
			modelID = cfg.TranscribeModel
		}
	}
	if modelID == "" {
		modelID = "sense-voice"
	}

	modelDir := filepath.Join(audioModelsDir(), modelID)
	if _, err := os.Stat(modelDir); err != nil {
		return fmt.Errorf("model %q not found at %s\nRun 'aigc-cli audio init --model %s' to download it", modelID, modelDir, modelID)
	}

	if shared.Verbose {
		fmt.Printf("Local ASR: model=%s file=%s\n", modelID, input)
	}

	start := time.Now()
	engine, err := audio.NewASREngine("", modelDir)
	if err != nil {
		return fmt.Errorf("load ASR model: %w", err)
	}
	defer engine.Close()

	text, err := engine.Transcribe(input)
	if err != nil {
		return fmt.Errorf("ASR inference: %w", err)
	}
	elapsed := time.Since(start)

	if shared.Verbose {
		fmt.Printf("Duration: %.1fs\n", elapsed.Seconds())
	}

	// Save transcription to file
	filename, err := saveTranscriptionFile(text, input)
	if err != nil {
		return fmt.Errorf("failed to save transcription: %w", err)
	}
	fmt.Printf("Saved: %s\n", filename)
	return nil
}
