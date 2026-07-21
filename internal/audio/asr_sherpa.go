package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

func NewASREngine(_, modelDir string) (*ASREngine, error) {
	if err := loadSherpa(); err != nil {
		return nil, err
	}
	// Validate model exists
	enc := findFileFFI(modelDir, "encoder.onnx")
	dec := filepath.Join(filepath.Dir(enc), "decoder.onnx")
	mp := findFileFFI(modelDir, "model.onnx")
	tok := findFileFFI(modelDir, "tokens.txt")

	if tok == "" {
		return nil, fmt.Errorf("tokens.txt not found in %s", modelDir)
	}

	var impl unsafe.Pointer
	if enc != "" {
		if _, err := os.Stat(dec); err == nil {
			impl = ffi.AsrCreate(cstr(enc), cstr(dec), cstr(tok), nil, 1)
		}
	} else if mp != "" {
		impl = ffi.AsrCreate(nil, nil, cstr(tok), cstr(mp), 0)
	}
	if impl == nil {
		return nil, fmt.Errorf("no compatible ASR model found in %s", modelDir)
	}

	return &ASREngine{impl: impl, modelDir: modelDir}, nil
}

func (e *ASREngine) Transcribe(path string) (string, error) {
	if e.impl == nil {
		return "", fmt.Errorf("ASR engine not initialized")
	}

	stream := ffi.AsrCreateStream(e.impl)
	if stream == nil {
		return "", fmt.Errorf("failed to create ASR stream")
	}
	defer ffi.AsrDestroyStream(stream)

	if strings.HasSuffix(strings.ToLower(path), ".wav") {
		wave := ffi.WaveRead(cstr(path))
		if wave == nil {
			return "", fmt.Errorf("failed to read: %s", path)
		}
		defer ffi.WaveDestroy(wave)

		sr := ffi.WaveSampleRate(wave)
		n := ffi.WaveNumSamples(wave)
		samples := ffi.WaveSamples(wave)
		if samples != nil {
			ffi.AsrAcceptWaveform(stream, sr, samples, n)
		}
	} else {
		data, err := DecodeAudioFile(path)
		if err != nil {
			return "", fmt.Errorf("decode: %w", err)
		}
		s := make([]float32, len(data.Samples))
		for i, v := range data.Samples {
			s[i] = float32(v) / 32767
		}
		if len(s) > 0 {
			ffi.AsrAcceptWaveform(stream, int32(data.SampleRate), &s[0], int32(len(s)))
		}
	}

	ffi.AsrDecode(e.impl, stream)
	text := gostr(ffi.AsrGetText(stream))
	return text, nil
}

func (e *ASREngine) Close() {
	if e.impl != nil {
		ffi.AsrDestroy(e.impl)
		e.impl = nil
	}
}
