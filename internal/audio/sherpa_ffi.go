package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

// openLibrary is defined in platform-specific files:
// loadlib.go (darwin/linux) - purego.Dlopen
// loadlib_windows.go (windows) - windows.LoadLibrary

var ffi struct {
	lib uintptr

	TtsCreateVits      func(model, tokens, lexicon *byte) unsafe.Pointer
	TtsCreateKokoro    func(model, tokens, voices, dataDir, lexicon *byte) unsafe.Pointer
	TtsCreateMatcha    func(acoustic, vocoder, tokens, lexicon, dataDir *byte) unsafe.Pointer
	TtsDestroy         func(tts unsafe.Pointer)
	TtsGenerate        func(tts unsafe.Pointer, text *byte, sid int32, speed float32) unsafe.Pointer
	TtsNumSpeakers     func(tts unsafe.Pointer) int32
	TtsFreeAudio       func(audio unsafe.Pointer)
	TtsAudioSamples    func(audio unsafe.Pointer) *float32
	TtsAudioN          func(audio unsafe.Pointer) int32
	TtsAudioSampleRate func(audio unsafe.Pointer) int32

	AsrCreate         func(enc, dec, toks, model *byte, isWhisper int32) unsafe.Pointer
	AsrDestroy        func(rec unsafe.Pointer)
	AsrCreateStream   func(rec unsafe.Pointer) unsafe.Pointer
	AsrDestroyStream  func(s unsafe.Pointer)
	AsrAcceptWaveform func(s unsafe.Pointer, sr int32, samples *float32, n int32)
	AsrDecode         func(rec, s unsafe.Pointer)
	AsrGetText        func(s unsafe.Pointer) *byte

	WaveRead       func(path *byte) unsafe.Pointer
	WaveDestroy    func(w unsafe.Pointer)
	WaveSamples    func(w unsafe.Pointer) *float32
	WaveSampleRate func(w unsafe.Pointer) int32
	WaveNumSamples func(w unsafe.Pointer) int32
}

func loadSherpa() error {
	if ffi.lib != 0 {
		return nil
	}
	lib := findHelperLib()
	if lib == "" || filepath.Dir(lib) == "." {
		// Either not found, or found as bare name (which won't work on Windows)
		helperName := map[string]string{
			"darwin":  "libaigc-sherpa-helper.dylib",
			"linux":   "libaigc-sherpa-helper.so",
			"windows": "aigc-sherpa-helper.dll",
		}[runtime.GOOS]
		return fmt.Errorf("local audio not configured\nPlace %s in one of these directories:\n  - %s\n  - %s\n  - %s\nThen run 'aigc-cli audio init'",
			helperName,
			exeDir(),
			filepath.Join(audioConfigDir(), "models"),
			filepath.Join(audioConfigDir(), "models", "audio"))
	}
	h, err := openLibrary(lib)
	if err != nil {
		helperDir := filepath.Dir(lib)

		// Check only the current platform's dependencies.
		expectedLibs := map[string][]string{
			"windows": {"sherpa-onnx-c-api.dll", "onnxruntime.dll"},
			"darwin":  {"libsherpa-onnx-c-api.dylib", "libonnxruntime.1.27.0.dylib"},
			"linux":   {"libsherpa-onnx-c-api.so", "libonnxruntime.so"},
		}
		depErr := ""
		for _, dep := range expectedLibs[runtime.GOOS] {
			if _, statErr := os.Stat(filepath.Join(helperDir, dep)); statErr != nil {
				depErr += fmt.Sprintf("    missing: %s\n", dep)
			}
		}
		if depErr != "" {
			return fmt.Errorf("audio helper found in %s\nbut dependencies missing:\n%sExtract ALL files from the release ZIP into that directory", helperDir, depErr)
		}
		return fmt.Errorf("load audio helper (%s): %w", lib, err)
	}
	ffi.lib = h
	r := func(fn any, name string) { purego.RegisterLibFunc(fn, h, name) }
	r(&ffi.TtsCreateVits, "tts_create_vits")
	r(&ffi.TtsCreateKokoro, "tts_create_kokoro")
	r(&ffi.TtsCreateMatcha, "tts_create_matcha")
	r(&ffi.TtsDestroy, "tts_destroy")
	r(&ffi.TtsGenerate, "tts_generate")
	r(&ffi.TtsNumSpeakers, "tts_num_speakers")
	r(&ffi.TtsFreeAudio, "tts_free_audio")
	r(&ffi.TtsAudioSamples, "tts_audio_samps")
	r(&ffi.TtsAudioN, "tts_audio_n")
	r(&ffi.TtsAudioSampleRate, "tts_audio_sr")
	r(&ffi.AsrCreate, "asr_create")
	r(&ffi.AsrDestroy, "asr_destroy")
	r(&ffi.AsrCreateStream, "asr_create_stream")
	r(&ffi.AsrDestroyStream, "asr_destroy_stream")
	r(&ffi.AsrAcceptWaveform, "asr_accept_waveform")
	r(&ffi.AsrDecode, "asr_decode")
	r(&ffi.AsrGetText, "asr_get_text")
	r(&ffi.WaveRead, "wave_read")
	r(&ffi.WaveDestroy, "wave_destroy")
	r(&ffi.WaveSamples, "wave_samps")
	r(&ffi.WaveSampleRate, "wave_sr")
	r(&ffi.WaveNumSamples, "wave_n")
	return nil
}

func libSearchPaths() []string {
	return []string{exeDir(), ".", filepath.Join(audioConfigDir(), "models"),
		filepath.Join(audioConfigDir(), "models", "audio")}
}

func findHelperLib() string {
	names := map[string][]string{
		"darwin":  {"libaigc-sherpa-helper.dylib"},
		"linux":   {"libaigc-sherpa-helper.so"},
		"windows": {"aigc-sherpa-helper.dll", "libaigc-sherpa-helper.dll"},
	}[runtime.GOOS]
	if len(names) == 0 {
		return ""
	}
	for _, d := range libSearchPaths() {
		for _, name := range names {
			p := filepath.Join(d, name)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return names[0]
}

func audioConfigDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ".config/aigc-cli"
	}
	return filepath.Join(home, ".config", "aigc-cli")
}

func exeDir() string {
	e, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(e)
}

func cstr(s string) *byte {
	if s == "" {
		return nil
	}
	b := append([]byte(s), 0)
	return &b[0]
}

func gostr(p *byte) string {
	if p == nil {
		return ""
	}
	// Find length via null terminator
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), uintptr(n))) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

func float32Slice(p *float32, n int) []float32 {
	if p == nil || n <= 0 {
		return nil
	}
	return unsafe.Slice(p, n)
}
