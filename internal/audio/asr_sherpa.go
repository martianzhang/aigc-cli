//go:build cgo

package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

func NewASREngine(_, modelDir string) (*ASREngine, error) {
	cfg := asrConfig(modelDir)
	if cfg.ModelConfig.Whisper.Encoder == "" && cfg.ModelConfig.SenseVoice.Model == "" {
		return nil, fmt.Errorf("no compatible ASR model found in %s", modelDir)
	}
	return &ASREngine{modelDir: modelDir}, nil
}

func (e *ASREngine) Transcribe(path string) (string, error) {
	cfg := asrConfig(e.modelDir)
	rec := sherpa.NewOfflineRecognizer(&cfg)
	if rec == nil {
		return "", fmt.Errorf("failed to create recognizer")
	}
	defer sherpa.DeleteOfflineRecognizer(rec)

	stream := sherpa.NewOfflineStream(rec)
	if stream == nil {
		return "", fmt.Errorf("failed to create stream")
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
			return "", fmt.Errorf("decode: %w", err)
		}
		s := make([]float32, len(data.Samples))
		for i, v := range data.Samples {
			s[i] = float32(v) / 32767
		}
		stream.AcceptWaveform(data.SampleRate, s)
	}

	rec.Decode(stream)
	result := stream.GetResult()
	if result == nil {
		return "", fmt.Errorf("recognition returned nil")
	}
	return result.Text, nil
}

func (e *ASREngine) Close() {}

func asrConfig(dir string) sherpa.OfflineRecognizerConfig {
	cfg := sherpa.OfflineRecognizerConfig{
		FeatConfig:     sherpa.FeatureConfig{SampleRate: 16000, FeatureDim: 80},
		ModelConfig:    sherpa.OfflineModelConfig{Debug: 0},
		DecodingMethod: "greedy_search",
	}
	if encoder := findFile(dir, "encoder.onnx"); encoder != "" {
		decoder := filepath.Join(filepath.Dir(encoder), "decoder.onnx")
		if _, err := os.Stat(decoder); err == nil {
			cfg.ModelConfig.Tokens = filepath.Join(filepath.Dir(encoder), "tokens.txt")
			cfg.ModelConfig.Whisper = sherpa.OfflineWhisperModelConfig{
				Encoder: encoder, Decoder: decoder, Language: "", Task: "transcribe",
			}
		}
	}
	if cfg.ModelConfig.Whisper.Encoder == "" {
		if mp := findFile(dir, "model.onnx"); mp != "" {
			cfg.ModelConfig.Tokens = filepath.Join(filepath.Dir(mp), "tokens.txt")
			cfg.ModelConfig.SenseVoice = sherpa.OfflineSenseVoiceModelConfig{
				Model: mp, Language: "",
			}
		}
	}
	return cfg
}
