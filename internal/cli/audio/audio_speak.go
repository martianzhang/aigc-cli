package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/audio"
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
)

func runAudioSpeak(cmd *cobra.Command, args []string) error {
	// ── Local mode: -P local, type=local, or no provider+model ──
	p := options.Shared.ResolveProvider(options.ProviderNameAudioSpeak)
	if isLocalAudioMode(p) {
		return runLocalAudioSpeak(cmd)
	}

	req, err := buildAudioSpeechRequest()
	if err != nil {
		return err
	}

	if cfg := options.AudioDefaults(); cfg != nil {
		if req.Model == "" {
			req.Model = cfg.SpeakModel()
		}
		if req.Voice == "" {
			req.Voice = cfg.SpeakVoice()
		}
		if req.ResponseFormat == "" {
			req.ResponseFormat = cfg.SpeakFormat()
		}
	}

	if req.Model == "" {
		req.Model = "gpt-4o-mini-tts"
	}
	if req.Voice == "" {
		return fmt.Errorf("voice is required: set via --voice flag")
	}
	if req.ResponseFormat == "" {
		req.ResponseFormat = "mp3"
	}
	if req.Speed == 0 {
		req.Speed = 1.0
	}

	if audioDryRun {
		fmt.Println(buildAudioSpeechCurl(req))
		return nil
	}

	if options.Shared.Verbose {
		fmt.Printf("Request: model=%s voice=%s format=%s speed=%.1f\n",
			req.Model, req.Voice, req.ResponseFormat, req.Speed)
		if req.Instructions != "" {
			fmt.Printf("Instructions: %s\n", req.Instructions)
		}
		fmt.Printf("Input length: %d chars\n", len(req.Input))
	}

	c := client.NewFromProvider(p)
	options.ApplyTimeout(c, "audio", client.AudioTimeout)

	start := time.Now()
	audioData, contentType, err := c.AudioSpeech(req)
	if err != nil {
		return fmt.Errorf("TTS failed: %w", err)
	}
	elapsed := time.Since(start)

	actualFormat := req.ResponseFormat
	if contentType != "" {
		actualFormat = audioFormatFromContentType(contentType)
	}

	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Voice: %s\n", req.Voice)
	fmt.Printf("Format: %s\n", actualFormat)
	fmt.Printf("Size: %d bytes\n", len(audioData))
	fmt.Printf("Duration: %.1fs\n", elapsed.Seconds())

	filename, err := saveAudioFile(audioData, actualFormat)
	if err != nil {
		return fmt.Errorf("failed to save audio: %w", err)
	}
	fmt.Printf("Saved: %s\n", filename)

	if audioSpeechPlay {
		if err := service.PreviewFile(filename); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: playback failed: %v\n", err)
		}
	}
	return nil
}

// runLocalAudioSpeak handles local TTS inference via sherpa-onnx.
func runLocalAudioSpeak(_ *cobra.Command) error {
	req, err := buildAudioSpeechRequest()
	if err != nil {
		return err
	}

	modelID := audioSpeechModel
	if modelID == "" {
		if cfg := options.AudioDefaults(); cfg != nil {
			modelID = cfg.SpeakModel()
		}
	}
	if modelID == "" {
		modelID = "kokoro" // best balance for mixed EN/ZH
	}

	if audioDryRun {
		voice := audioSpeechVoice
		if voice == "" {
			if cfg := options.AudioDefaults(); cfg != nil {
				voice = cfg.SpeakVoice()
			}
		}
		if voice == "" {
			voice = "default"
		}
		printLocalDryRun("audio speak", []localDryRunField{
			{"model", modelID},
			{"voice", fmt.Sprintf("%s (sid=%d)", voice, resolveSID(audioSpeechVoice))},
			{"format", "wav"},
			{"input", fmt.Sprintf("%d chars", len(req.Input))},
			{"output", plannedAudioOutput("wav")},
		})
		return nil
	}

	modelDir := filepath.Join(audioModelsDir(), modelID)
	if _, err := os.Stat(modelDir); err != nil {
		return fmt.Errorf("model %q not found at %s\nRun 'aigc-cli audio init --model %s' to download it", modelID, modelDir, modelID)
	}

	if options.Shared.Verbose {
		fmt.Printf("Local mode: model=%s input=%d chars\n", modelID, len(req.Input))
	}

	sid := resolveSID(audioSpeechVoice)

	start := time.Now()
	engine, err := audio.NewTTSEngine("", modelDir)
	if err != nil {
		return fmt.Errorf("load TTS model: %w", err)
	}
	defer engine.Close()

	pcm, sampleRate, err := engine.Speak(req.Input, sid)
	if err != nil {
		return fmt.Errorf("TTS inference: %w", err)
	}
	elapsed := time.Since(start)

	fmt.Printf("Model: %s (local)\n", modelID)
	fmt.Printf("Format: wav\n")
	fmt.Printf("Duration: %.1fs (%.1f sec audio)\n", elapsed.Seconds(), float64(len(pcm))/float64(sampleRate))

	wavData := &audio.AudioData{Samples: pcm, SampleRate: sampleRate}
	filename, err := saveAudioWAV(wavData)
	if err != nil {
		return fmt.Errorf("failed to save audio: %w", err)
	}
	fmt.Printf("Saved: %s\n", filename)

	if audioSpeechPlay {
		if err := service.PreviewFile(filename); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: playback failed: %v\n", err)
		}
	}
	return nil
}

// saveAudioWAV saves PCM audio as a WAV file to the output directory.
func saveAudioWAV(data *audio.AudioData) (string, error) {
	dir := options.Shared.OutputDir
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}
	filename := filepath.Join(dir, fmt.Sprintf("audio_%d.wav", time.Now().Unix()))
	if err := audio.WriteWAV(filename, data); err != nil {
		return "", fmt.Errorf("write WAV: %w", err)
	}
	return filename, nil
}

// resolveSID parses a voice string as either a named Kokoro voice or numeric SID.
// Falls back to config.defaults.audio.voice, then DefaultKokoroVoice.
func resolveSID(voiceFlag string) int {
	v := voiceFlag
	if v == "" {
		if cfg := options.AudioDefaults(); cfg != nil {
			v = cfg.SpeakVoice()
		}
	}
	if v == "" {
		return audio.DefaultKokoroVoice
	}
	// Try name lookup first
	if sid, ok := audio.KokoroVoiceNames[v]; ok {
		return sid
	}
	// Try numeric
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	// Unknown name (e.g. cloud voice like "alloy") → use default
	return audio.DefaultKokoroVoice
}
