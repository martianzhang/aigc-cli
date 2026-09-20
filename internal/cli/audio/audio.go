package audio

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/audio"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

var audioCmd = &cobra.Command{
	Use:     "audio",
	Aliases: []string{"voice"},
	Short:   "Audio operations (also: voice)",
	Long: `Generate speech from text (TTS) or transcribe audio to text (STT).

Supports OpenAI, OpenRouter, and APIMart providers with automatic detection.
All providers use the OpenAI-compatible endpoints. Local inference via --local
uses ONNX models downloaded with 'audio init'.

Subcommands:
  speak / tts       Convert text to speech audio
  play              Play an audio file (no external app needed)
  transcribe / asr  Convert audio to text
  init              Download local audio models`,
	Example: `  aigc-cli audio speak --input "Hello world" --voice alloy
  aigc-cli audio speak --input text.txt --voice nova --play
  aigc-cli audio transcribe --input recording.wav
  aigc-cli audio init --list`,
}

var speechCmd = &cobra.Command{
	Use:          "speak",
	Aliases:      []string{"tts"},
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

var playCmd = &cobra.Command{
	Use:          "play <file>",
	Short:        "Play an audio file through speakers",
	SilenceUsage: true,
	Long: `Play an audio file using Go's built-in audio decoder (no external app required).

Supports WAV, MP3, FLAC, and OGG/Vorbis formats.

Examples:
  aigc-cli audio play recording.wav
  aigc-cli audio play audio_1784644392.wav`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("file not found: %s", path)
		}
		fmt.Fprintf(os.Stderr, "Playing...\n")
		if err := audio.PlayAudioFile(path); err != nil {
			return fmt.Errorf("playback failed: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Done.\n")
		return nil
	},
}

var transcribeCmd = &cobra.Command{
	Use:          "transcribe",
	Aliases:      []string{"asr", "stt"},
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
	audioSpeechLocal        bool

	// Transcribe flags
	audioTranscribeModel       string
	audioTranscribeInput       string
	audioTranscribeFormat      string
	audioTranscribeLanguage    string
	audioTranscribeTemperature float64
	audioTranscribeDryRun      bool
	audioTranscribeLocal       bool
)

// isLocalAudioMode returns true if audio should run as local inference.
// Local mode is triggered by the --local flag, or when there is no
// online provider configured (nil, type=local, or empty name+model).
func isLocalAudioMode(p *provider.EffectiveProvider, localFlag bool) bool {
	return localFlag || p == nil || p.Type == types.ProviderLocal || (p.Name == "" && p.Model == "")
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
	f.StringVarP(&audioSpeechModel, "model", "m", "", "TTS model (cloud: model name / local: model ID)")
	f.StringVarP(&audioSpeechVoice, "voice", "V", "", "Voice name (e.g. alloy, nova, echo, fable)")
	f.StringVarP(&audioSpeechFormat, "format", "f", "", "Audio format: mp3, wav, opus, aac, flac, pcm (default: mp3)")
	f.Float64VarP(&audioSpeechSpeed, "speed", "s", 0, "Playback speed: 0.25-4.0 (default: 1.0)")
	f.StringVar(&audioSpeechInstructions, "instructions", "", "Tone/voice instructions (OpenAI gpt-4o-mini-tts only)")
	f.BoolVar(&audioSpeechDryRun, "dry-run", false, "Print curl command without calling API")
	f.BoolVar(&audioSpeechPlay, "play", false, "Play audio with system default player after generation")
	f.BoolVar(&audioSpeechLocal, "local", false, "Use local TTS model instead of cloud API")
}

func registerAudioTranscribeFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVarP(&audioTranscribeInput, "input", "i", "", "Audio file path or omit to auto-detect piped base64 stdin")
	f.StringVarP(&audioTranscribeModel, "model", "m", "", "STT model (cloud: model name / local: model ID)")
	f.StringVar(&audioTranscribeFormat, "format", "", "Audio format: wav, mp3, flac, m4a, ogg (auto-detected from file extension)")
	f.StringVarP(&audioTranscribeLanguage, "language", "l", "", "Language hint (ISO-639-1, e.g. en, ja, zh)")
	f.Float64Var(&audioTranscribeTemperature, "temperature", 0, "Sampling temperature 0-1 (default: 0)")
	f.BoolVar(&audioTranscribeDryRun, "dry-run", false, "Print curl command without calling API")
	f.BoolVar(&audioTranscribeLocal, "local", false, "Use local ASR model instead of cloud API")
}

func init() {
	registerAudioSpeakFlags(speechCmd)
	registerAudioTranscribeFlags(transcribeCmd)
	audioCmd.AddCommand(speechCmd)
	audioCmd.AddCommand(playCmd)
	audioCmd.AddCommand(transcribeCmd)
}

// Cmd returns the audio command tree.
func Cmd() *cobra.Command { return audioCmd }
