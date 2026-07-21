package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

type TTSEngine struct {
	tts *sherpa.OfflineTts
	sr  int
}

func NewTTSEngine(libPath, modelDir string) (*TTSEngine, error) {
	info, err := resolveModel(modelDir)
	if err != nil {
		return nil, err
	}
	cfg := sherpa.OfflineTtsConfig{
		Model: sherpa.OfflineTtsModelConfig{NumThreads: 2, Debug: 0},
	}
	switch info.engine {
	case "kokoro":
		cfg.Model.Kokoro = sherpa.OfflineTtsKokoroModelConfig{
			Model: info.model, Tokens: info.tokens, Voices: info.voices,
			DataDir: info.dataDir, Lexicon: info.lexicon,
		}
	case "matcha":
		cfg.Model.Matcha = sherpa.OfflineTtsMatchaModelConfig{
			AcousticModel: info.model, Vocoder: info.vocoder,
			Tokens: info.tokens, Lexicon: info.lexicon, DataDir: info.dataDir,
		}
	default:
		cfg.Model.Vits = sherpa.OfflineTtsVitsModelConfig{
			Model: info.model, Tokens: info.tokens, Lexicon: info.lexicon,
		}
	}
	tts := sherpa.NewOfflineTts(&cfg)
	if tts == nil {
		return nil, fmt.Errorf("failed to create TTS engine")
	}
	return &TTSEngine{tts: tts, sr: info.sr}, nil
}

func (e *TTSEngine) Speak(text string, sid int) ([]int16, int, error) {
	audio := e.tts.Generate(text, sid, 1.0)
	if audio == nil {
		return nil, 0, fmt.Errorf("TTS returned nil")
	}
	samples := make([]int16, len(audio.Samples))
	for i, v := range audio.Samples {
		s := v * 32767
		if s > 32767 {
			s = 32767
		} else if s < -32768 {
			s = -32768
		}
		samples[i] = int16(s)
	}
	e.sr = audio.SampleRate
	return samples, audio.SampleRate, nil
}

func (e *TTSEngine) NumSpeakers() int {
	if e.tts != nil {
		return e.tts.NumSpeakers()
	}
	return 0
}

func (e *TTSEngine) Close() {
	if e.tts != nil {
		sherpa.DeleteOfflineTts(e.tts)
	}
}

type resolvedModel struct {
	engine  string
	model   string
	tokens  string
	voices  string
	vocoder string
	dataDir string
	lexicon string
	sr      int
}

func resolveModel(dir string) (*resolvedModel, error) {
	// Kokoro: has voices.bin
	if vp := findFile(dir, "voices.bin"); vp != "" {
		base := filepath.Dir(vp)
		return &resolvedModel{
			engine: "kokoro", sr: 24000,
			model:   filepath.Join(base, "model.onnx"),
			tokens:  filepath.Join(base, "tokens.txt"),
			voices:  vp,
			dataDir: filepath.Join(base, "espeak-ng-data"),
			lexicon: filepath.Join(base, "lexicon-us-en.txt") + "," + filepath.Join(base, "lexicon-zh.txt"),
		}, nil
	}

	// Matcha: has onnx file that is NOT named "model.onnx" (e.g. model-steps-3.onnx)
	// and a vocoder in the shared audio models directory
	if mp := findAcousticModel(dir); mp != "" {
		base := filepath.Dir(mp)
		r := &resolvedModel{
			engine: "matcha", model: mp, sr: 22050,
			tokens:  filepath.Join(base, "tokens.txt"),
			dataDir: filepath.Join(base, "espeak-ng-data"),
		}
		if lp := filepath.Join(base, "lexicon.txt"); fileExists(lp) {
			r.lexicon = lp
		}
		// Vocoder: shared location at audio/_vocoder/vocos.onnx
		audioBase := filepath.Dir(dir)
		r.vocoder = filepath.Join(audioBase, "_vocoder", "vocos.onnx")
		return r, nil
	}

	// VITS: standard model.onnx
	mp := findFile(dir, "model.onnx")
	if mp == "" {
		return nil, fmt.Errorf("model.onnx not found in %s", dir)
	}
	base := filepath.Dir(mp)
	r := &resolvedModel{engine: "vits", model: mp, tokens: filepath.Join(base, "tokens.txt"), sr: 22050}
	if lp := filepath.Join(base, "lexicon.txt"); fileExists(lp) {
		r.lexicon = lp
	}
	return r, nil
}

// findAcousticModel looks for Matcha-style model files (*.onnx but not model.onnx).
func findAcousticModel(dir string) string {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			sub, _ := os.ReadDir(filepath.Join(dir, e.Name()))
			for _, f := range sub {
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".onnx") && f.Name() != "model.onnx" {
					return filepath.Join(dir, e.Name(), f.Name())
				}
			}
		}
	}
	return ""
}

func findFile(dir, name string) string {
	if fileExists(filepath.Join(dir, name)) {
		return filepath.Join(dir, name)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			if p := filepath.Join(dir, e.Name(), name); fileExists(p) {
				return p
			}
		}
	}
	return ""
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
