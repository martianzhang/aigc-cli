package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

// ASREngine wraps sherpa-onnx's OfflineRecognizer for local speech recognition.
type ASREngine struct {
	recognizer *sherpa.OfflineRecognizer
}

// NewASREngine creates an ASR engine from a model directory.
// Detects model type (Whisper, SenseVoice, etc.) from files present.
func NewASREngine(libPath, modelDir string) (*ASREngine, error) {
	cfg := sherpa.OfflineRecognizerConfig{
		FeatConfig: sherpa.FeatureConfig{
			SampleRate: 16000,
			FeatureDim: 80,
		},
		ModelConfig:    sherpa.OfflineModelConfig{Debug: 0},
		DecodingMethod: "greedy_search",
	}

	// Detect model type from files
	if encoder := findFile(modelDir, "encoder.onnx"); encoder != "" {
		decoder := filepath.Join(filepath.Dir(encoder), "decoder.onnx")
		if _, err := os.Stat(decoder); err == nil {
			cfg.ModelConfig.Tokens = filepath.Join(filepath.Dir(encoder), "tokens.txt")
			cfg.ModelConfig.Whisper = sherpa.OfflineWhisperModelConfig{
				Encoder:  encoder,
				Decoder:  decoder,
				Language: "",
				Task:     "transcribe",
			}
		}
	}

	// Fallback: single model.onnx → SenseVoice or Paraformer
	if cfg.ModelConfig.Whisper.Encoder == "" {
		if mp := findFile(modelDir, "model.onnx"); mp != "" {
			cfg.ModelConfig.Tokens = filepath.Join(filepath.Dir(mp), "tokens.txt")
			cfg.ModelConfig.SenseVoice = sherpa.OfflineSenseVoiceModelConfig{
				Model:    mp,
				Language: "auto",
			}
		}
	}

	// Validate
	if cfg.ModelConfig.Whisper.Encoder == "" && cfg.ModelConfig.SenseVoice.Model == "" {
		return nil, fmt.Errorf("no compatible ASR model found in %s", modelDir)
	}

	rec := sherpa.NewOfflineRecognizer(&cfg)
	if rec == nil {
		return nil, fmt.Errorf("failed to create offline recognizer")
	}

	return &ASREngine{recognizer: rec}, nil
}

// Transcribe transcribes an audio file and returns the recognized text.
// Supports WAV, MP3, FLAC, and OGG formats.
func (e *ASREngine) Transcribe(path string) (string, error) {
	stream := sherpa.NewOfflineStream(e.recognizer)
	if stream == nil {
		return "", fmt.Errorf("failed to create offline stream")
	}
	defer sherpa.DeleteOfflineStream(stream)

	if strings.HasSuffix(strings.ToLower(path), ".wav") {
		wave := sherpa.ReadWave(path)
		if wave == nil {
			return "", fmt.Errorf("failed to read WAV: %s", path)
		}
		stream.AcceptWaveform(wave.SampleRate, wave.Samples)
	} else {
		data, err := DecodeAudioFile(path)
		if err != nil {
			return "", fmt.Errorf("decode audio: %w", err)
		}
		// Convert int16 PCM to float32 for sherpa
		samples := make([]float32, len(data.Samples))
		for i, s := range data.Samples {
			samples[i] = float32(s) / 32767
		}
		stream.AcceptWaveform(data.SampleRate, samples)
	}

	e.recognizer.Decode(stream)
	result := stream.GetResult()
	if result == nil {
		return "", fmt.Errorf("recognition returned nil")
	}
	return result.Text, nil
}

// Close releases ASR engine resources.
func (e *ASREngine) Close() {
	if e.recognizer != nil {
		sherpa.DeleteOfflineRecognizer(e.recognizer)
	}
}
