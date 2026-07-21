package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func NewTTSEngine(_, modelDir string) (*TTSEngine, error) {
	if err := loadSherpa(); err != nil {
		return nil, err
	}
	info, err := resolveModelFFI(modelDir)
	if err != nil {
		return nil, err
	}
	e := &TTSEngine{sr: info.sr, modelDir: modelDir}
	switch info.engine {
	case "kokoro":
		e.impl = ffi.TtsCreateKokoro(
			cstr(info.model), cstr(info.tokens), cstr(info.voices),
			cstr(info.dataDir), cstr(info.lexicon))

	case "matcha":
		e.impl = ffi.TtsCreateMatcha(
			cstr(info.model), cstr(info.vocoder),
			cstr(info.tokens), cstr(info.lexicon), cstr(info.dataDir))

	default:
		e.impl = ffi.TtsCreateVits(
			cstr(info.model), cstr(info.tokens), cstr(info.lexicon))
	}
	if e.impl == nil {
		return nil, fmt.Errorf("failed to create TTS engine (%s)", info.engine)
	}
	return e, nil
}

func (e *TTSEngine) NumSpeakers() int {
	if e.impl == nil {
		return 0
	}
	return int(ffi.TtsNumSpeakers(e.impl))
}

func (e *TTSEngine) Speak(text string, sid int) ([]int16, int, error) {
	if e.impl == nil {
		return nil, 0, fmt.Errorf("TTS engine not initialized")
	}
	audio := ffi.TtsGenerate(e.impl, cstr(text), int32(sid), 1.0)
	if audio == nil {
		return nil, 0, fmt.Errorf("TTS returned nil")
	}
	defer ffi.TtsFreeAudio(audio)

	n := int(ffi.TtsAudioN(audio))
	sr := int(ffi.TtsAudioSampleRate(audio))
	samples := float32Slice(ffi.TtsAudioSamples(audio), n)

	out := make([]int16, n)
	for i, v := range samples {
		s := v * 32767
		if s > 32767 {
			s = 32767
		} else if s < -32768 {
			s = -32768
		}
		out[i] = int16(s)
	}
	e.sr = sr
	return out, sr, nil
}

func (e *TTSEngine) Close() {
	if e.impl != nil {
		ffi.TtsDestroy(e.impl)
		e.impl = nil
	}
}

func resolveModelFFI(dir string) (*resolvedModel, error) {
	if vp := findFileFFI(dir, "voices.bin"); vp != "" {
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
	if mp := findAcousticModelFFI(dir); mp != "" {
		base := filepath.Dir(mp)
		r := &resolvedModel{engine: "matcha", model: mp, sr: 22050, tokens: filepath.Join(base, "tokens.txt")}
		r.dataDir = filepath.Join(base, "espeak-ng-data")
		if lp := filepath.Join(base, "lexicon.txt"); fileExistsFFI(lp) {
			r.lexicon = lp
		}
		r.vocoder = filepath.Join(filepath.Dir(dir), "_vocoder", "vocos.onnx")
		return r, nil
	}
	mp := findFileFFI(dir, "model.onnx")
	if mp == "" {
		return nil, fmt.Errorf("model.onnx not found in %s", dir)
	}
	base := filepath.Dir(mp)
	r := &resolvedModel{engine: "vits", model: mp, tokens: filepath.Join(base, "tokens.txt"), sr: 22050}
	if lp := filepath.Join(base, "lexicon.txt"); fileExistsFFI(lp) {
		r.lexicon = lp
	}
	return r, nil
}

func findAcousticModelFFI(dir string) string {
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

func findFileFFI(dir, name string) string {
	if fileExistsFFI(filepath.Join(dir, name)) {
		return filepath.Join(dir, name)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			if p := filepath.Join(dir, e.Name(), name); fileExistsFFI(p) {
				return p
			}
		}
	}
	return ""
}

func fileExistsFFI(p string) bool {
	_, err := os.Stat(p)
	return err == nil
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
