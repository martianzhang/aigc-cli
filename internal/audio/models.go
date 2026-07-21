// Package audio provides model registry and local inference for audio
// processing (TTS and ASR) using ONNX models with pure-onnx runtime.
//
// Models are downloaded via `aigc-cli audio init --model <id>` and stored in
// ~/.config/aigc-cli/models/audio/<id>/.
package audio

import (
	"fmt"
	"os"
)

// ModelType distinguishes ASR (speech-to-text) from TTS (text-to-speech).
type ModelType string

const (
	ModelASR ModelType = "asr"
	ModelTTS ModelType = "tts"
)

// ModelFile describes a single file that makes up a model.
type ModelFile struct {
	URL      string // download source (HTTP/HTTPS)
	Path     string // relative path within the model directory, e.g. "model.onnx"
	Optional bool   // if true, skip download when the URL is unreachable
}

// TTSBackend selects which sherpa-onnx model config to use.
type TTSBackend string

const (
	TTSVits   TTSBackend = "vits"
	TTSKokoro TTSBackend = "kokoro"
	TTSMatcha TTSBackend = "matcha"
)

// ModelInfo holds metadata and download URLs for an audio model.
// Each model directory is stored at:
//
//	~/.config/aigc-cli/models/audio/<ID>/
type ModelInfo struct {
	ID          string      // unique identifier, e.g. "kokoro"
	Name        string      // human-readable name, e.g. "Kokoro 82M"
	Type        ModelType   // ASR or TTS
	Backend     TTSBackend  // TTS engine type (vits, kokoro), empty for ASR
	Description string      // one-line description
	Language    []string    // supported languages (BCP-47 codes)
	Size        string      // approximate download size, e.g. "130MB"
	SampleRate  int         // expected audio sample rate
	Files       []ModelFile // files to download
	NeedToken   bool        // whether HuggingFace token is required
	License     string      // SPDX identifier or custom, e.g. "Apache-2.0", "CC BY-NC 4.0"
}

// Registry returns all known audio models indexed by ID.
func Registry() map[string]ModelInfo {
	return map[string]ModelInfo{
		"matcha-zh-en": {
			ID:          "matcha-zh-en",
			Name:        "Matcha-TTS 中英双语",
			Type:        ModelTTS,
			Backend:     TTSMatcha,
			Description: "Matcha-TTS bilingual Chinese-English (IceFall), natural bilingual speech",
			Language:    []string{"zh", "en"},
			Size:        "125MB",
			SampleRate:  22050,
			Files: []ModelFile{
				{URL: "https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/matcha-icefall-zh-en.tar.bz2", Path: "model.tar.bz2"},
				{URL: "https://github.com/k2-fsa/sherpa-onnx/releases/download/vocoder-models/vocos-22khz-univ.onnx", Path: "../_vocoder/vocos.onnx"},
			},
			NeedToken: false,
			License:   "MIT",
		},
		"matcha-en": {
			ID:          "matcha-en",
			Name:        "Matcha-TTS English",
			Type:        ModelTTS,
			Backend:     TTSMatcha,
			Description: "Matcha-TTS American English (LJSpeech), high quality",
			Language:    []string{"en"},
			Size:        "125MB",
			SampleRate:  22050,
			Files: []ModelFile{
				{URL: "https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/matcha-icefall-en_US-ljspeech.tar.bz2", Path: "model.tar.bz2"},
				{URL: "https://github.com/k2-fsa/sherpa-onnx/releases/download/vocoder-models/vocos-22khz-univ.onnx", Path: "../_vocoder/vocos.onnx"},
			},
			NeedToken: false,
			License:   "MIT",
		},
		"vits-zh-ll": {
			ID:          "vits-zh-ll",
			Name:        "VITS 中文",
			Type:        ModelTTS,
			Backend:     TTSVits,
			Description: "VITS Chinese TTS, 5 speakers, uses Bopomofo phonemes via lexicon",
			Language:    []string{"zh"},
			Size:        "115MB",
			SampleRate:  22050,
			Files: []ModelFile{
				{
					URL:  "https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/sherpa-onnx-vits-zh-ll.tar.bz2",
					Path: "model.tar.bz2",
				},
			},
			NeedToken: false,
			License:   "MIT",
		},
		"kokoro": {
			ID:          "kokoro",
			Name:        "Kokoro 82M",
			Type:        ModelTTS,
			Backend:     TTSKokoro,
			Description: "Multilingual TTS, 82M params, 53 voices. EN/ZH/JA/KO/FR",
			Language:    []string{"en", "zh", "ja", "ko", "fr"},
			Size:        "130MB",
			SampleRate:  24000,
			Files: []ModelFile{
				{URL: "https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/kokoro-multi-lang-v1_0.tar.bz2", Path: "model.tar.bz2"},
			},
			NeedToken: false,
			License:   "Apache-2.0",
		},
		"kokoro-en": {
			ID:          "kokoro-en",
			Name:        "Kokoro English",
			Type:        ModelTTS,
			Backend:     TTSKokoro,
			Description: "Kokoro English-only, 82M params, same quality, smaller download",
			Language:    []string{"en"},
			Size:        "100MB",
			SampleRate:  24000,
			Files: []ModelFile{
				{URL: "https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/kokoro-en-v0_19.tar.bz2", Path: "model.tar.bz2"},
			},
			NeedToken: false,
			License:   "Apache-2.0",
		},
		"vits-zh-hf-eula": {
			ID:          "vits-zh-hf-eula",
			Name:        "VITS 中文 Eula",
			Type:        ModelTTS,
			Backend:     TTSVits,
			Description: "Chinese VITS, 804 speakers (HuggingFace Eula dataset), natural Chinese",
			Language:    []string{"zh"},
			Size:        "116MB",
			SampleRate:  22050,
			Files: []ModelFile{
				{URL: "https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/vits-zh-hf-eula.tar.bz2", Path: "model.tar.bz2"},
			},
			NeedToken: false,
			License:   "MIT",
		},
		"vits-zh-aishell3": {
			ID:          "vits-zh-aishell3",
			Name:        "VITS 中文 AISHELL-3",
			Type:        ModelTTS,
			Backend:     TTSVits,
			Description: "Chinese VITS, single female speaker (AISHELL-3 dataset), clear standard Mandarin",
			Language:    []string{"zh"},
			Size:        "115MB",
			SampleRate:  22050,
			Files: []ModelFile{
				{URL: "https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/vits-zh-aishell3.tar.bz2", Path: "model.tar.bz2"},
			},
			NeedToken: false,
			License:   "MIT",
		},
		"vits-cantonese": {
			ID:          "vits-cantonese",
			Name:        "VITS 粤语",
			Type:        ModelTTS,
			Backend:     TTSVits,
			Description: "Cantonese VITS (xiaomaiiwn dataset), Hong Kong-style Cantonese TTS",
			Language:    []string{"yue"},
			Size:        "115MB",
			SampleRate:  22050,
			Files: []ModelFile{
				{URL: "https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/vits-cantonese-hf-xiaomaiiwn.tar.bz2", Path: "model.tar.bz2"},
			},
			NeedToken: false,
			License:   "MIT",
		},
		"vits-ljs": {
			ID:          "vits-ljs",
			Name:        "VITS LJSpeech",
			Type:        ModelTTS,
			Backend:     TTSVits,
			Description: "VITS American English TTS, LJSpeech dataset (single speaker, female), US accent",
			Language:    []string{"en"},
			Size:        "117MB",
			SampleRate:  22050,
			Files: []ModelFile{
				{
					URL:  "https://huggingface.co/csukuangfj/vits-ljs/resolve/main/vits-ljs.onnx",
					Path: "model.onnx",
				},
				{
					URL:  "https://huggingface.co/csukuangfj/vits-ljs/resolve/main/tokens.txt",
					Path: "tokens.txt",
				},
				{
					URL:  "https://huggingface.co/csukuangfj/vits-ljs/resolve/main/lexicon.txt",
					Path: "lexicon.txt",
				},
			},
			NeedToken: false,
			License:   "MIT",
		},
		"vits-vctk": {
			ID:          "vits-vctk",
			Name:        "VITS VCTK",
			Type:        ModelTTS,
			Backend:     TTSVits,
			Description: "VITS English multi-speaker TTS, VCTK corpus (109 speakers, British accent)",
			Language:    []string{"en"},
			Size:        "117MB",
			SampleRate:  22050,
			Files: []ModelFile{
				{
					URL:  "https://huggingface.co/csukuangfj/vits-vctk/resolve/main/vits-vctk.onnx",
					Path: "model.onnx",
				},
				{
					URL:  "https://huggingface.co/csukuangfj/vits-vctk/resolve/main/tokens.txt",
					Path: "tokens.txt",
				},
				{
					URL:  "https://huggingface.co/csukuangfj/vits-vctk/resolve/main/lexicon.txt",
					Path: "lexicon.txt",
				},
			},
			NeedToken: false,
			License:   "MIT",
		},
		"whisper-tiny": {
			ID:          "whisper-tiny",
			Name:        "Whisper Tiny",
			Type:        ModelASR,
			Description: "OpenAI Whisper Tiny, 39M params, multilingual ASR",
			Language:    []string{"en", "zh", "ja", "ko", "fr", "de", "es"},
			Size:        "150MB",
			SampleRate:  16000,
			Files: []ModelFile{
				{
					URL:  "https://huggingface.co/csukuangfj/sherpa-onnx-whisper-tiny/resolve/main/tiny-encoder.onnx",
					Path: "encoder.onnx",
				},
				{
					URL:  "https://huggingface.co/csukuangfj/sherpa-onnx-whisper-tiny/resolve/main/tiny-decoder.onnx",
					Path: "decoder.onnx",
				},
				{
					URL:  "https://huggingface.co/csukuangfj/sherpa-onnx-whisper-tiny/resolve/main/tiny-tokens.txt",
					Path: "tokens.txt",
				},
			},
			NeedToken: false,
			License:   "Apache-2.0",
		},
		"sense-voice": {
			ID:          "sense-voice",
			Name:        "SenseVoice",
			Type:        ModelASR,
			Description: "Alibaba SenseVoice, 80M params, Chinese & multilingual ASR (best for Chinese)",
			Language:    []string{"zh", "en", "yue", "ja", "ko"},
			Size:        "80MB",
			SampleRate:  16000,
			Files: []ModelFile{
				{URL: "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-sense-voice-zh-en-ja-ko-yue-2024-07-17.tar.bz2", Path: "model.tar.bz2"},
			},
			NeedToken: false,
			License:   "MIT",
		},
	}
}

// Lookup returns the model with the given ID, or an error if not found.
func Lookup(id string) (ModelInfo, error) {
	m, ok := Registry()[id]
	if !ok {
		return ModelInfo{}, fmt.Errorf("unknown model %q", id)
	}
	return m, nil
}

// DefaultKokoroVoice is the default speaker ID for Kokoro TTS (zf_xiaoxiao = Chinese female).
const DefaultKokoroVoice = 47

// KokoroVoiceNames maps Kokoro voice names to speaker IDs (official).
// Prefix: a=US EN, b=GB EN, e=ES, f=FR, h=HI, i=IT, j=JA, p=PT, z=ZH
// Suffix: f=female, m=male
// Source: https://k2-fsa.github.io/sherpa/onnx/tts/all/Chinese-English/kokoro-multi-lang-v1_0.html
var KokoroVoiceNames = map[string]int{
	"af_alloy": 0, "af_aoede": 1, "af_bella": 2, "af_heart": 3,
	"af_jessica": 4, "af_kore": 5, "af_nicole": 6, "af_nova": 7,
	"af_river": 8, "af_sarah": 9, "af_sky": 10,
	"am_adam": 11, "am_echo": 12, "am_eric": 13, "am_fenrir": 14,
	"am_liam": 15, "am_michael": 16, "am_onyx": 17, "am_puck": 18, "am_santa": 19,
	"bf_alice": 20, "bf_emma": 21, "bf_isabella": 22, "bf_lily": 23,
	"bm_daniel": 24, "bm_fable": 25, "bm_george": 26, "bm_lewis": 27,
	"ef_dora": 28, "em_alex": 29,
	"ff_siwis": 30,
	"hf_alpha": 31, "hf_beta": 32, "hm_omega": 33, "hm_psi": 34,
	"if_sara": 35, "im_nicola": 36,
	"jf_alpha": 37, "jf_gongitsune": 38, "jf_nezumi": 39, "jf_tebukuro": 40,
	"jm_kumo": 41,
	"pf_dora": 42, "pm_alex": 43, "pm_santa": 44,
	"zf_xiaobei": 45, "zf_xiaoni": 46, "zf_xiaoxiao": 47, "zf_xiaoyi": 48,
	"zm_yunjian": 49, "zm_yunxi": 50, "zm_yunxia": 51, "zm_yunyang": 52,
}

// ListByType returns all models of the given type (or all if typ is empty),
// optionally filtered by language.
func ListByType(typ ModelType, lang string) []ModelInfo {
	var out []ModelInfo
	for _, m := range Registry() {
		if typ != "" && m.Type != typ {
			continue
		}
		if lang != "" && !contains(m.Language, lang) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// ListInstalled returns models whose directories exist under the given base path.
func ListInstalled(baseDir string) ([]ModelInfo, error) {
	var out []ModelInfo
	for _, m := range Registry() {
		modelDir := dirFor(m.ID, baseDir)
		ok, err := dirExists(modelDir)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// dirFor returns the directory for the given model ID under baseDir/models/audio/.
func dirFor(id, baseDir string) string {
	return baseDir + "/audio/" + id
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func dirExists(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return fi.IsDir(), nil
}
