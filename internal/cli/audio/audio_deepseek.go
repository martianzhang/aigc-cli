package audio

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/reqbuild"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// deepSeekASRMaxBytes is the ASR upload cap (8 MiB) enforced by the API.
const deepSeekASRMaxBytes = 8 << 20

// deepSeekTTSMaxTextBytes is the TTS request body limit (64 KiB).
const deepSeekTTSMaxTextBytes = 64 << 10

// runDeepSeekAudioSpeak synthesises speech with POST /v1/tools/tts. Its voice
// set (stella/tide) and formats (wav/pcm) differ from OpenAI's, so the shared
// request is validated and re-defaulted here instead of being sent as-is.
func runDeepSeekAudioSpeak(cmd *cobra.Command, p *provider.EffectiveProvider, req *types.AudioSpeechRequest) error {
	voice, err := normalizeDeepSeekVoice(req.Voice)
	if err != nil {
		// A voice configured for another provider (e.g. a local Kokoro voice)
		// must not break DeepSeek; only an explicit --voice is an error.
		if cmd != nil && !cmd.Flags().Changed("voice") {
			voice = provider.DeepSeekDefaultVoice
		} else {
			return err
		}
	}
	format, err := normalizeDeepSeekTTSFormat(req.ResponseFormat)
	if err != nil {
		// Likewise, a global OpenAI format (e.g. mp3) falls back to wav.
		if cmd != nil && !cmd.Flags().Changed("format") {
			format = "wav"
		} else {
			return err
		}
	}

	if cmd != nil {
		for _, name := range []string{"model", "speed", "instructions"} {
			flag := cmd.Flags().Lookup(name)
			if flag != nil && flag.Changed {
				ui.Eprintf("⚠ --%s has no effect on DeepSeek TTS\n", name)
			}
		}
	}
	if len(req.Input) > deepSeekTTSMaxTextBytes {
		return fmt.Errorf("DeepSeek TTS accepts at most 64 KiB of text per request (got %d bytes)", len(req.Input))
	}

	if audioDryRun {
		body := map[string]any{
			"text":     req.Input,
			"voice_id": voice,
			"format":   format,
		}
		plan := &reqbuild.Plan{
			Method:    http.MethodPost,
			URL:       provider.DeepSeekToolURL(p.BaseURL, "tts"),
			Body:      body,
			ExtraArgs: []string{"--output speech." + format},
		}
		ui.Println(plan.RenderCurls(p.APIKey))
		return nil
	}

	if options.Shared.Verbose {
		ui.Printf("Request: endpoint=%s voice=%s format=%s\n",
			provider.DeepSeekToolURL(p.BaseURL, "tts"), voice, format)
		ui.Printf("Input length: %d chars\n", len(req.Input))
	}

	start := time.Now()
	audioData, _, err := provider.DeepSeekTTS(p, req.Input, voice, format, nil)
	if err != nil {
		return fmt.Errorf("TTS failed: %w", err)
	}
	elapsed := time.Since(start)

	ui.Printf("Model: deepseek-tts\n")
	ui.Printf("Voice: %s (%s)\n", voice, deepSeekVoiceLabel(voice))
	ui.Printf("Format: %s\n", format)
	ui.Printf("Size: %d bytes\n", len(audioData))
	ui.Printf("Duration: %.1fs\n", elapsed.Seconds())

	filename, err := saveAudioFile(audioData, format)
	if err != nil {
		return fmt.Errorf("failed to save audio: %w", err)
	}
	ui.Printf("Saved: %s\n", filename)
	printAudioResult([]string{filename}, "deepseek-tts", p.ProviderType.String())

	if audioSpeechPlay {
		if err := service.PreviewFile(filename); err != nil {
			ui.Eprintf("Warning: playback failed: %v\n", err)
		}
	}
	return nil
}

// runDeepSeekAudioTranscribe uploads one audio file to POST /v1/tools/asr.
// DeepSeek accepts 16-bit PCM WAV or Ogg Opus only, with an 8 MiB / 120s cap,
// and detects the language automatically.
func runDeepSeekAudioTranscribe(cmd *cobra.Command, p *provider.EffectiveProvider, input string) error {
	if err := validateDeepSeekAudioInput(cmd, input); err != nil {
		return err
	}

	if audioDryRun {
		plan := &reqbuild.Plan{
			Method:    http.MethodPost,
			URL:       provider.DeepSeekToolURL(p.BaseURL, "asr"),
			Multipart: true,
			FormFiles: []reqbuild.FormFile{{Field: "file", Path: input}},
		}
		ui.Println(plan.RenderCurls(p.APIKey))
		return nil
	}

	if options.Shared.Verbose {
		ui.Printf("Uploading file: %s\n", input)
	}

	start := time.Now()
	result, err := provider.DeepSeekTranscribe(p, input)
	if err != nil {
		return fmt.Errorf("STT failed: %w", err)
	}
	elapsed := time.Since(start)

	ui.Printf("Model: deepseek-asr\n")
	ui.Printf("Duration: %.1fs\n", elapsed.Seconds())
	if result.DurationMS > 0 {
		ui.Printf("Audio: %.1fs\n", float64(result.DurationMS)/1000)
	}

	filename, err := saveTranscriptionFile(result.Text, input)
	if err != nil {
		return fmt.Errorf("failed to save transcription: %w", err)
	}
	ui.Printf("Saved: %s\n", filename)
	printAudioResult([]string{filename}, "deepseek-asr", p.ProviderType.String())
	return nil
}

// normalizeDeepSeekVoice validates a voice id, defaulting to stella.
func normalizeDeepSeekVoice(voice string) (string, error) {
	voice = strings.ToLower(strings.TrimSpace(voice))
	if voice == "" {
		return provider.DeepSeekDefaultVoice, nil
	}
	switch voice {
	case "stella", "tide":
		return voice, nil
	default:
		return "", fmt.Errorf("unsupported DeepSeek voice %q: use stella (female) or tide (male)", voice)
	}
}

// normalizeDeepSeekTTSFormat validates an audio format, defaulting to wav.
func normalizeDeepSeekTTSFormat(format string) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return "wav", nil
	}
	switch format {
	case "wav", "pcm":
		return format, nil
	default:
		return "", fmt.Errorf("DeepSeek TTS supports only wav or pcm (got %q); use --format wav", format)
	}
}

func deepSeekVoiceLabel(voice string) string {
	if voice == "tide" {
		return "male"
	}
	return "female"
}

// validateDeepSeekAudioInput rejects inputs the ASR endpoint cannot accept and
// warns about flags it ignores, before any bytes are uploaded.
func validateDeepSeekAudioInput(cmd *cobra.Command, input string) error {
	if !service.IsFile(input) {
		return fmt.Errorf("DeepSeek ASR requires a local audio file (16-bit PCM WAV or Ogg Opus); base64/stdin input is not supported")
	}

	ext := strings.ToLower(filepath.Ext(input))
	switch ext {
	case ".wav", ".ogg", ".oga", ".opus":
	default:
		return fmt.Errorf("DeepSeek ASR accepts only 16-bit PCM WAV or Ogg Opus (got %q)", ext)
	}

	info, err := os.Stat(input)
	if err != nil {
		return fmt.Errorf("read audio file: %w", err)
	}
	if info.Size() > deepSeekASRMaxBytes {
		return fmt.Errorf("audio file is %d MiB; DeepSeek ASR accepts at most 8 MiB", (info.Size()+(1<<20)-1)>>20)
	}

	if cmd != nil {
		for _, name := range []string{"model", "language", "format"} {
			flag := cmd.Flags().Lookup(name)
			if flag != nil && flag.Changed {
				ui.Eprintf("⚠ --%s has no effect on DeepSeek ASR (language is auto-detected)\n", name)
			}
		}
	}
	return nil
}
