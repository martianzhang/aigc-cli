package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/audio"
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/onnxrt"
)

var audioInitCmd = &cobra.Command{
	Use:          "init",
	Short:        "Download audio models for local inference",
	SilenceUsage: true,
	Long: `Download ONNX audio models for local TTS and speech recognition.

Models are saved to ~/.config/aigc-cli/models/audio/<model-id>/.

The ONNX Runtime is shared with the 'detect' and 'background' commands.
If not already installed, it will be downloaded automatically.

TTS models:
  kokoro          Multilingual (EN/ZH/JA/KO/FR), 82M params, ~130MB (default)
  kokoro-en       English-only Kokoro, 100MB
  vits-zh-ll      Chinese VITS, 5 speakers, 115MB
  vits-zh-hf-eula Chinese VITS, 804 speakers (natural Chinese), 116MB
  vits-zh-aishell3 Chinese VITS, female, 115MB
  vits-cantonese  Cantonese VITS, 115MB
  vits-ljs        American English VITS (LJSpeech), 115MB
  vits-vctk       British English VITS (VCTK, 109 speakers), 115MB

ASR models:
  whisper-tiny    OpenAI Whisper Tiny, 39M params, ~150MB
  sense-voice     Alibaba SenseVoice, 80M params, ~80MB
  whisper-tiny    OpenAI Whisper Tiny ASR, 39M params, ~150MB
  sense-voice     Alibaba SenseVoice ASR, 80M params, ~80MB

Use --list to see all available models. Use --list-installed to see what
you already have. Proxy settings are automatically respected.`,
	Example: `  aigc-cli audio init                                    # download defaults: kokoro (TTS) + sense-voice (ASR)
  aigc-cli audio init --list                             # list available models
  aigc-cli audio init --model kokoro --model sense-voice
  aigc-cli audio init --list-voices --model kokoro`,
	RunE: runAudioInit,
}

var (
	audioInitModel      []string
	audioInitList       bool
	audioInitListInst   bool
	audioInitType       string
	audioInitLang       string
	audioInitForce      bool
	audioInitHFToken    string
	audioInitURL        string
	audioInitName       string
	audioInitListVoices bool
)

func runAudioInit(cmd *cobra.Command, args []string) error {
	modelsDir := audioModelsDir()

	// ── List available models ──
	if audioInitList {
		typ := audio.ModelType(audioInitType)
		if typ != "" && typ != audio.ModelASR && typ != audio.ModelTTS {
			return fmt.Errorf("invalid type %q (choose: asr, tts)", audioInitType)
		}
		models := audio.ListByType(typ, audioInitLang)
		if len(models) == 0 {
			fmt.Println("No models found matching the criteria.")
			return nil
		}
		fmt.Printf("Available models (type=%s lang=%s):\n", audioInitType, audioInitLang)
		for _, m := range models {
			fmt.Printf("  %-16s  %-10s  %-8s  %s\n", m.ID, m.Type, m.Size, m.Description)
		}
		return nil
	}

	// ── List installed models ──
	if audioInitListInst {
		installed, err := audio.ListInstalled(filepath.Dir(audioModelsDir()))
		if err != nil {
			return fmt.Errorf("list installed: %w", err)
		}
		if len(installed) == 0 {
			fmt.Println("No audio models installed. Run 'aigc-cli audio init --model <id>' to download one.")
			return nil
		}
		fmt.Println("Installed audio models:")
		for _, m := range installed {
			fmt.Printf("  %-16s  %-10s  %-8s  %s\n", m.ID, m.Type, m.Size, m.Description)
		}
		return nil
	}

	// ── List voices for a model ──
	if audioInitListVoices {
		if len(audioInitModel) == 0 {
			return fmt.Errorf("specify a model with --model to list its voices")
		}
		modelID := audioInitModel[0]
		modelDir := filepath.Join(audioModelsDir(), modelID)
		if _, err := os.Stat(modelDir); err != nil {
			return fmt.Errorf("model %q not installed, run 'audio init --model %s' first", modelID, modelID)
		}
		engine, err := audio.NewTTSEngine("", modelDir)
		if err != nil {
			return fmt.Errorf("load model: %w", err)
		}
		count := engine.NumSpeakers()
		engine.Close()
		if count <= 0 {
			fmt.Printf("Model %q has 1 voice\n", modelID)
			return nil
		}
		fmt.Printf("Model %q has %d voices (SID 0-%d)\n", modelID, count, count-1)

		names := voiceNamesForModel(modelID, count)
		if len(names) > 0 {
			fmt.Println("\nNamed voices:")
			type vs struct {
				sid  int
				name string
			}
			var sorted []vs
			for sid, name := range names {
				sorted = append(sorted, vs{sid, name})
			}
			sort.Slice(sorted, func(i, j int) bool { return sorted[i].sid < sorted[j].sid })
			for _, v := range sorted {
				fmt.Printf("  %-4d  %s\n", v.sid, v.name)
			}
		} else {
			fmt.Printf("Use --voice <SID> to select a voice (0-%d)\n", count-1)
		}
		return nil
	}

	// ── Download from URL (custom model) ──
	if audioInitURL != "" {
		name := audioInitName
		if name == "" {
			name = "custom-model"
		}
		if err := downloadFromURL(audioInitURL, audioModelsDir(), name, audioInitForce); err != nil {
			return fmt.Errorf("download: %w", err)
		}
		fmt.Printf("Custom model installed as %q.\n", name)
		return nil
	}

	// ── Download models ──
	if len(audioInitModel) == 0 {
		audioInitModel = []string{"kokoro", "sense-voice"}
		fmt.Println("No model specified, downloading defaults: kokoro (TTS) + sense-voice (ASR)")
	}

	// Ensure ONNX Runtime is installed
	if _, err := onnxrt.EnsureInstalled(filepath.Dir(audioModelsDir()), audioInitForce); err != nil {
		return err
	}

	// Ensure sherpa-onnx runtime libraries are available
	if err := ensureAudioRuntime(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: audio runtime not fully installed: %v\n", err)
		fmt.Fprintf(os.Stderr, "  Local TTS/ASR will not work. Use a release build or install GCC and run:\n")
		fmt.Fprintf(os.Stderr, "    bash scripts/build-helper.sh\n")
	}

	for _, modelID := range audioInitModel {
		info, err := audio.Lookup(modelID)
		if err != nil {
			return err
		}
		if err := downloadModelFiles(info, modelsDir, audioInitForce); err != nil {
			return fmt.Errorf("model %q: %w", modelID, err)
		}
		fmt.Printf("Model %q installed. Use 'aigc-cli audio speak --local --input \"...\"' to try it.\n", modelID)
	}
	return nil
}

func init() {
	audioCmd.AddCommand(audioInitCmd)
	audioInitCmd.Flags().StringSliceVar(&audioInitModel, "model", nil, "model ID(s) to download (repeatable: --model a --model b)")
	audioInitCmd.Flags().BoolVar(&audioInitList, "list", false, "list available models")
	audioInitCmd.Flags().BoolVar(&audioInitListInst, "list-installed", false, "list installed models")
	audioInitCmd.Flags().StringVar(&audioInitType, "type", "", "filter by type: asr, tts")
	audioInitCmd.Flags().StringVar(&audioInitLang, "lang", "", "filter by language code (e.g. zh, en)")
	audioInitCmd.Flags().BoolVar(&audioInitForce, "force", false, "re-download even if already installed")
	audioInitCmd.Flags().StringVar(&audioInitHFToken, "hf-token", "", "HuggingFace token for gated models")
	audioInitCmd.Flags().StringVar(&audioInitURL, "url", "", "download from arbitrary URL (use with --name)")
	audioInitCmd.Flags().StringVar(&audioInitName, "name", "", "model name for --url downloads")
	audioInitCmd.Flags().BoolVar(&audioInitListVoices, "list-voices", false, "list available voices for a model")
}

// voiceNamesForModel returns known voice names for a given model, or nil if unknown.
// Currently only kokoro has a complete name mapping. Other models use raw SIDs.
func voiceNamesForModel(modelID string, count int) map[int]string {
	switch modelID {
	case "kokoro", "kokoro-en":
		names := make(map[int]string)
		for name, sid := range audio.KokoroVoiceNames {
			if sid < count {
				names[sid] = name
			}
		}
		return names
	}
	return nil
}

// audioModelsDir returns the base directory for audio models.
// Default: ~/.config/aigc-cli/models/audio/
func audioModelsDir() string {
	// Use shared models directory if available from detect config
	if cfg := options.DetectConfig(); cfg != nil && cfg.ModelsDir != "" {
		return filepath.Join(cfg.ModelsDir, "audio")
	}
	if cfg := options.BackgroundConfig(); cfg != nil && cfg.ModelsDir != "" {
		return filepath.Join(cfg.ModelsDir, "audio")
	}
	return filepath.Join(options.ConfigDir(), "models", "audio")
}
