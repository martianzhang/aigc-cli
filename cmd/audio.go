package cmd

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

var (
	// Speak flags
	audioSpeechModel        string
	audioSpeechInput        string
	audioSpeechVoice        string
	audioSpeechFormat       string
	audioSpeechSpeed        float64
	audioSpeechInstructions string
	audioSpeechDryRun       bool
	audioSpeechPlay         bool

	// Transcribe flags
	audioTranscribeModel       string
	audioTranscribeInput       string
	audioTranscribeFormat      string
	audioTranscribeLanguage    string
	audioTranscribeTemperature float64
	audioTranscribeDryRun      bool
)

var audioCmd = &cobra.Command{
	Use:     "audio",
	Aliases: []string{"voice"},
	Short:   "Audio operations (also: voice)",
	Long: `Generate speech from text (TTS) or transcribe audio to text (STT).

Supports OpenAI, OpenRouter, and APIMart providers with automatic detection.
All providers use the OpenAI-compatible endpoints.

Subcommands:
  speak       Convert text to speech audio
  transcribe  Convert audio to text`,
}

var speechCmd = &cobra.Command{
	Use:          "speak",
	Short:        "Convert text to speech audio",
	SilenceUsage: true,
	Long: `Convert text to spoken audio using AI TTS models.

Input can be provided via --input flag, file path, or piped stdin (auto-detected).
The audio response is saved as a file in the output directory.

Examples:
  aigc-cli audio speak --model gpt-4o-mini-tts --input "Hello world" --voice alloy
  aigc-cli audio speak --model gpt-4o-mini-tts --input text.txt --voice nova
  echo "Hello" | aigc-cli audio speak --model gpt-4o-mini-tts --voice alloy
  aigc-cli audio speak --model gpt-4o-mini-tts --input "Hi" --voice alloy --format wav --speed 1.2`,
	RunE: runAudioSpeak,
}

var transcribeCmd = &cobra.Command{
	Use:          "transcribe",
	Short:        "Convert audio to text",
	SilenceUsage: true,
	Long: `Transcribe audio files to text using AI STT models.

Input can be a local audio file path, or piped base64 data via stdin (auto-detected).
Large files are sent as multipart/form-data; other input uses JSON body.

Examples:
  aigc-cli audio transcribe --model whisper-1 --input recording.wav
  aigc-cli audio transcribe --model whisper-1 --input speech.mp3 --language en
  cat recording.wav | base64 | aigc-cli audio transcribe --model whisper-1 --format wav`,
	RunE: runAudioTranscribe,
}

func runAudioSpeak(cmd *cobra.Command, args []string) error {
	req, err := buildAudioSpeechRequest()
	if err != nil {
		return err
	}

	if cfg, err := config.LoadDefaults(shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil && cfg.Defaults.Audio != nil {
		if req.Model == "" {
			req.Model = cfg.Defaults.Audio.SpeakModel
		}
		if req.Voice == "" && cfg.Defaults.Audio.Voice != "" {
			req.Voice = cfg.Defaults.Audio.Voice
		}
		if req.ResponseFormat == "" && cfg.Defaults.Audio.Format != "" {
			req.ResponseFormat = cfg.Defaults.Audio.Format
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

	if audioSpeechDryRun {
		fmt.Println(buildAudioSpeechCurl(req))
		return nil
	}

	if shared.Verbose {
		fmt.Printf("Request: model=%s voice=%s format=%s speed=%.1f\n",
			req.Model, req.Voice, req.ResponseFormat, req.Speed)
		if req.Instructions != "" {
			fmt.Printf("Instructions: %s\n", req.Instructions)
		}
		fmt.Printf("Input length: %d chars\n", len(req.Input))
	}

	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
	applyTimeout(c, "audio", client.AudioTimeout)

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

func runAudioTranscribe(cmd *cobra.Command, args []string) error {
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

	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
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

		filename, err := saveTranscriptionFile(sttResp.Text)
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

	filename, err := saveTranscriptionFile(sttResp.Text)
	if err != nil {
		return fmt.Errorf("failed to save transcription: %w", err)
	}
	fmt.Printf("Saved: %s\n", filename)
	return nil
}

func detectAudioFormat(path string) string {
	ext := ""
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			ext = path[i+1:]
			break
		}
	}
	switch ext {
	case "wav", "mp3", "flac", "m4a", "ogg", "webm", "aac":
		return ext
	default:
		return "wav"
	}
}

func registerAudioSpeakFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVarP(&audioSpeechInput, "input", "i", "", "Text input (file path, raw text, or omit to auto-detect piped stdin)")
	f.StringVarP(&audioSpeechModel, "model", "m", "", "TTS model (e.g. openai/gpt-4o-mini-tts)")
	f.StringVarP(&audioSpeechVoice, "voice", "V", "", "Voice name (e.g. alloy, nova, echo, fable)")
	f.StringVarP(&audioSpeechFormat, "format", "f", "", "Audio format: mp3, wav, opus, aac, flac, pcm (default: mp3)")
	f.Float64VarP(&audioSpeechSpeed, "speed", "s", 0, "Playback speed: 0.25-4.0 (default: 1.0)")
	f.StringVar(&audioSpeechInstructions, "instructions", "", "Tone/voice instructions (OpenAI gpt-4o-mini-tts only)")
	f.BoolVar(&audioSpeechDryRun, "dry-run", false, "Print curl command without calling API")
	f.BoolVar(&audioSpeechPlay, "play", false, "Play audio with system default player after generation")
}

func registerAudioTranscribeFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVarP(&audioTranscribeInput, "input", "i", "", "Audio file path or omit to auto-detect piped base64 stdin")
	f.StringVarP(&audioTranscribeModel, "model", "m", "", "STT model (e.g. openai/whisper-1)")
	f.StringVar(&audioTranscribeFormat, "format", "", "Audio format: wav, mp3, flac, m4a, ogg (auto-detected from file extension)")
	f.StringVarP(&audioTranscribeLanguage, "language", "l", "", "Language hint (ISO-639-1, e.g. en, ja, zh)")
	f.Float64Var(&audioTranscribeTemperature, "temperature", 0, "Sampling temperature 0-1 (default: 0)")
	f.BoolVar(&audioTranscribeDryRun, "dry-run", false, "Print curl command without calling API")
}

func init() {
	registerAudioSpeakFlags(speechCmd)
	registerAudioTranscribeFlags(transcribeCmd)
	audioCmd.AddCommand(speechCmd)
	audioCmd.AddCommand(transcribeCmd)
	rootCmd.AddCommand(audioCmd)
}
